// Copyright 2026 OpenSSF Scorecard Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package provenance reports which Scorecard engine produced a result.
//
// Before the graft, cron lived inside ossf/scorecard, so the binary's own
// version was the engine's version and one value answered both questions.
// It does not any more: this binary is built from ossf/scorecard-infra and
// links ossf/scorecard as a dependency. The published record's
// `scorecard` object is the engine's -- api/openapi.yaml documents its
// `commit` as "SHA1 value of the Scorecard commit used for analysis", and
// docs/checks builds every check's documentation URL as
// github.com/ossf/scorecard/blob/<commit>/docs/checks.md, which only
// resolves for a ref that exists in that repository.
package provenance

import "runtime/debug"

// enginePath is the module whose version is reported. Kept as a constant
// rather than derived, so a major-version bump is a deliberate edit here.
const enginePath = "github.com/ossf/scorecard/v5"

// engineCommit is injected at link time; see the Makefile. A module version
// is all the build graph knows, and resolving it to a SHA needs the module
// proxy or the upstream repository -- neither of which a scanning worker
// should reach for at runtime. Empty when unset, which docs/checks treats
// the same as "unknown" and falls back to `main` for.
var engineCommit string

// Engine reports the version and commit of the Scorecard engine linked into
// this binary. Version comes from the build graph rather than a link flag so
// that it cannot disagree with the code actually running.
//
// Version is empty in a binary that does not link the engine at all, since
// debug.ReadBuildInfo lists only modules that made it into the build. It is
// also empty under `go test`: a test binary's BuildInfo carries no Deps at
// all, which is why the lookup below is split out and tested against a
// synthetic BuildInfo rather than the running one. `go version -m` on a built
// binary is how to check the real thing.
func Engine() (version, commit string) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", engineCommit
	}
	return engineVersionFrom(bi), engineCommit
}

func engineVersionFrom(bi *debug.BuildInfo) string {
	if bi == nil {
		return ""
	}
	for _, dep := range bi.Deps {
		if dep.Path != enginePath {
			continue
		}
		// A replace directive means the linked code is not what go.mod asks
		// for. Report what is actually there.
		if dep.Replace != nil {
			return dep.Replace.Version
		}
		return dep.Version
	}
	return ""
}
