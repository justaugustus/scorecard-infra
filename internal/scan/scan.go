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
// the SCM (GitHub/GitLab), OSS-Fuzz, CII, and vulnerabilities clients are
// created once and reused across scans; results are formatted to canonical JSON2
// via AsJSON2() and written back to the store to populate the cache.
//
// SCM API rate limits — not CPU — are the scaling bottleneck, so live scans are
// fronted by internal/tokens (token pool + per-host rate limiter) and bounded by
// an in-process worker pool.
package scan

import (
	"github.com/ossf/scorecard/v5/pkg/scorecard"
)

// runFn pins the pkg/scorecard entrypoint the Scanner adapts. Keeping a typed
// reference documents the exact upstream signature this package depends on and
// anchors the module dependency until the Scanner lands.
//
// TODO(group 4): implement Scanner (reused clients, JSON2 formatting, skip vs.
// fatal handling, write-back to the store) over this entrypoint.
var runFn = scorecard.Run

var _ = runFn
