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

package provenance

import (
	"regexp"
	"runtime/debug"
	"testing"
)

// A test binary's BuildInfo has no Deps -- confirmed, not assumed -- so
// engineVersionFrom is tested against a synthetic one. Reading the running
// binary's own info here would pass for the wrong reason: the lookup would
// return "" whether or not it worked.
func TestEngineVersionFrom(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		bi   *debug.BuildInfo
		name string
		want string
	}{
		{
			name: "NilBuildInfo",
			bi:   nil,
			want: "",
		},
		{
			name: "NoDeps",
			bi:   &debug.BuildInfo{},
			want: "",
		},
		{
			name: "EngineNotLinked",
			bi: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: "gocloud.dev", Version: "v0.46.0"},
			}},
			want: "",
		},
		{
			name: "EngineLinked",
			bi: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: "gocloud.dev", Version: "v0.46.0"},
				{Path: enginePath, Version: "v5.5.0"},
			}},
			want: "v5.5.0",
		},
		{
			// A replace makes go.mod's version a lie about what is linked.
			name: "EngineReplaced",
			bi: &debug.BuildInfo{Deps: []*debug.Module{
				{
					Path:    enginePath,
					Version: "v5.5.0",
					Replace: &debug.Module{Path: enginePath, Version: "v5.6.0-local"},
				},
			}},
			want: "v5.6.0-local",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			if got := engineVersionFrom(testcase.bi); got != testcase.want {
				t.Errorf("engineVersionFrom: got %q, want %q", got, testcase.want)
			}
		})
	}
}

// TestEngineCommitIsShapedLikeASHA guards the contract rather than a value:
// api/openapi.yaml constrains scorecard.commit to ^[0-9a-fA-F]{40}$, and
// docs/checks interpolates it straight into a github.com/ossf/scorecard URL.
// Empty is allowed -- an unstamped local build falls back to `main` -- but a
// half-set value would put a broken link on every check of every record.
func TestEngineCommitIsShapedLikeASHA(t *testing.T) {
	t.Parallel()

	_, commit := Engine()
	if commit == "" {
		t.Skip("engineCommit not injected; expected under a plain `go test`")
	}
	if !regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(commit) {
		t.Errorf("engine commit %q is not a 40-character hex SHA", commit)
	}
}
