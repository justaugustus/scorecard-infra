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

package server

import (
	"testing"

	"actionlint.kjanat.dev"
	"github.com/stretchr/testify/assert"
)

// The deployed parser already accepts cache-mode, so these diagnostics never
// occur against it. Each case pairs real YAML with the diagnostic an older
// parser produced (the workflow-scope message is verbatim from #5322), so the
// mechanism is exercised for when the next new key arrives ahead of a parser
// update.
const diagWorkflow = `name: Scorecard
on: push
permissions: read-all
cache-mode: none
jobs:
  analysis:
    runs-on: ubuntu-latest
    cache-mode: none
    steps:
      - uses: ossf/scorecard-action@fa7117db83c52d787ae5a89f350999701505d78e
`

func unexpectedKey(key, section string, line, col int) *actionlint.Error {
	return &actionlint.Error{
		Kind:    "syntax-check",
		Line:    line,
		Column:  col,
		Message: `unexpected key "` + key + `" for "` + section + `" section. expected one of "concurrency", "defaults"`,
	}
}

func TestUnreviewedDiagnostics(t *testing.T) {
	t.Parallel()
	readMode := `name: Scorecard
cache-mode: read
jobs: {}
`
	quoted := `name: Scorecard
cache-mode: "none"
jobs: {}
`
	tests := []struct {
		name    string
		content string
		diag    *actionlint.Error
		kept    bool
	}{
		{
			name:    "workflow-scope cache-mode: none is reviewed",
			content: diagWorkflow,
			diag:    unexpectedKey("cache-mode", "workflow", 4, 1),
		},
		{
			name:    "job-scope cache-mode: none is reviewed",
			content: diagWorkflow,
			diag:    unexpectedKey("cache-mode", "job", 8, 5),
		},
		{
			name:    "quoting doesn't change the value",
			content: quoted,
			diag:    unexpectedKey("cache-mode", "workflow", 2, 1),
		},
		{
			name:    "a value that keeps access is rejected",
			content: readMode,
			diag:    unexpectedKey("cache-mode", "workflow", 2, 1),
			kept:    true,
		},
		{
			name:    "an unreviewed key is rejected",
			content: diagWorkflow,
			diag:    unexpectedKey("permissions", "workflow", 3, 1),
			kept:    true,
		},
		{
			name:    "a reviewed key in an unreviewed section is rejected",
			content: diagWorkflow,
			diag:    unexpectedKey("cache-mode", "step", 8, 5),
			kept:    true,
		},
		{
			name:    "a position that doesn't hold the key is rejected",
			content: diagWorkflow,
			diag:    unexpectedKey("cache-mode", "workflow", 5, 1),
			kept:    true,
		},
		{
			name:    "a non-syntax diagnostic is rejected",
			content: diagWorkflow,
			diag: &actionlint.Error{
				Kind: "expression", Line: 4, Column: 1,
				Message: `unexpected key "cache-mode" for "workflow" section`,
			},
			kept: true,
		},
		{
			name:    "malformed YAML is rejected",
			content: "name: [unclosed\ncache-mode: none\n",
			diag:    unexpectedKey("cache-mode", "workflow", 2, 1),
			kept:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := unreviewedDiagnostics(tt.content, []*actionlint.Error{tt.diag})
			if tt.kept {
				assert.Equal(t, []*actionlint.Error{tt.diag}, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

// Only the reviewed diagnostic is dropped; anything alongside it survives.
func TestUnreviewedDiagnosticsKeepsTheRest(t *testing.T) {
	t.Parallel()
	reviewed := unexpectedKey("cache-mode", "workflow", 4, 1)
	other := unexpectedKey("not-a-key", "workflow", 2, 1)
	got := unreviewedDiagnostics(diagWorkflow, []*actionlint.Error{reviewed, other})
	assert.Equal(t, []*actionlint.Error{other}, got)
}

func TestUnreviewedDiagnosticsNone(t *testing.T) {
	t.Parallel()
	assert.Nil(t, unreviewedDiagnostics(diagWorkflow, nil))
}
