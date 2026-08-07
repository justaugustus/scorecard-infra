/*
Copyright 2026 The uwu-tools Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package scan generates Scorecard results on demand by wrapping
// pkg/scorecard.Run (design D8). Following the upstream ScorecardWorker pattern,
// the OSS-Fuzz, CII, and vulnerabilities clients are created once and reused
// across scans, while the SCM repo client is created per scan (it is stateful
// per repository and unsafe to share across concurrent scans). Results are
// formatted to canonical JSON2 via AsJSON2.
//
// The orchestrator (design D2) depends on the Scanner interface — not the
// concrete engine — so it is testable with a fake, and live scans are bounded by
// a Bounded worker pool and paced by internal/tokens.
//
// Coverage: the engine runs all default checks against any repository the
// configured SCM token can access (including private repos); it requires no
// opt-in. The server advertises this at /capabilities (design D7).
package scan

import (
	"context"
	"errors"

	"github.com/uwu-tools/scorecard-infra/internal/model"
)

// ErrSkipped indicates the repository could not be scanned for a non-fatal
// reason (unreachable, blocked, or behind an IP allow list). Callers treat it as
// "no result available", distinct from a scan failure (design task 4.3).
var ErrSkipped = errors.New("scan: repository skipped (unreachable or blocked)")

// Result is a produced Scorecard result. Field order is chosen for pointer
// packing (govet fieldalignment), not readability.
type Result struct {
	// ResolvedCommit is the commit SHA the scan actually ran against.
	ResolvedCommit string
	// JSON2 is the canonical Scorecard JSON2 body, served and cached verbatim.
	JSON2 []byte
	// Complete reports whether every check produced a conclusive score.
	Complete bool
}

// Scanner produces Scorecard results on demand. An empty commit means the
// repository's default-branch HEAD.
type Scanner interface {
	Scan(ctx context.Context, ref model.RepoRef, commit string) (*Result, error)
}

// Complete reports whether every check produced a conclusive score. A negative
// score (-1) is Scorecard's "inconclusive" sentinel, so its presence means the
// result is not fully complete. This one source-agnostic rule is applied to
// cached, live, and upstream results alike, so a token-limited local scan and an
// upstream result are judged by the same standard (design F4).
func Complete(checks []model.Check) bool {
	for i := range checks {
		if checks[i].Score < 0 {
			return false
		}
	}
	return true
}
