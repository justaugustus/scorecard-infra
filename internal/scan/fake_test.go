/*
Copyright 2026 OpenSSF Scorecard Authors.

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

package scan

import (
	"context"
	"errors"
	"testing"

	"github.com/ossf/scorecard-infra/internal/model"
)

func testRef(t *testing.T) model.RepoRef {
	t.Helper()
	ref, err := model.ParseRepoRef("github.com", "ossf", "scorecard")
	if err != nil {
		t.Fatalf("ParseRepoRef: %v", err)
	}
	return ref
}

func TestFakeScannerDefault(t *testing.T) {
	t.Parallel()

	f := &FakeScanner{}
	got, err := f.Scan(context.Background(), testRef(t), "deadbeef")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got.ResolvedCommit != "deadbeef" || !got.Complete || len(got.JSON2) == 0 {
		t.Errorf("default result = %+v, want echoed commit, complete, non-empty JSON2", got)
	}
	if f.Calls() != 1 {
		t.Errorf("Calls() = %d, want 1", f.Calls())
	}
}

func TestFakeScannerCustomFunc(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	f := &FakeScanner{
		ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*Result, error) {
			return nil, sentinel
		},
	}
	if _, err := f.Scan(context.Background(), testRef(t), ""); !errors.Is(err, sentinel) {
		t.Fatalf("Scan error = %v, want sentinel", err)
	}
}

func TestComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		checks []model.Check
		want   bool
	}{
		{name: "no checks is complete", checks: nil, want: true},
		{name: "all conclusive", checks: []model.Check{{Score: 10}, {Score: 0}}, want: true},
		{name: "one inconclusive", checks: []model.Check{{Score: 10}, {Score: -1}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Complete(tt.checks); got != tt.want {
				t.Errorf("Complete() = %v, want %v", got, tt.want)
			}
		})
	}
}
