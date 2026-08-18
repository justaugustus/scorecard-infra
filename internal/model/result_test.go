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

package model

import "testing"

// json2NoMetadata mirrors a Scorecard-webapp response: metadata omitted, details
// null. json2WithMetadata mirrors a pkg/scorecard.AsJSON2() body: metadata as a
// []string. Parse must handle both.
const (
	json2NoMetadata = `{
  "date": "2026-08-05T01:30:12Z",
  "repo": {"name": "github.com/ossf/scorecard", "commit": "2418d6d95e928102e1f3f8d6e7b92f4f3c78631f"},
  "scorecard": {"version": "v5.5.0", "commit": "c395761df6afe1a69e476bc60a013a94bcbc153f"},
  "score": 8.9,
  "checks": [
    {"name": "Dependency-Update-Tool", "score": 10, "reason": "update tool detected", "details": null,
     "documentation": {"short": "Determines if the project uses a dependency update tool.", "url": "https://x"}}
  ]
}`
	json2WithMetadata = `{
  "date": "2026-08-05T01:30:12Z",
  "repo": {"name": "github.com/ossf/scorecard", "commit": "2418d6d95e928102e1f3f8d6e7b92f4f3c78631f"},
  "scorecard": {"version": "v5.5.0", "commit": "c395761df6afe1a69e476bc60a013a94bcbc153f"},
  "score": 8.9,
  "checks": [],
  "metadata": ["example"]
}`
)

func TestParse(t *testing.T) {
	t.Parallel()

	r, err := Parse([]byte(json2NoMetadata))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if r.Date != "2026-08-05T01:30:12Z" {
		t.Errorf("Date = %q, want 2026-08-05T01:30:12Z", r.Date)
	}
	if r.Repo.Commit != "2418d6d95e928102e1f3f8d6e7b92f4f3c78631f" {
		t.Errorf("Repo.Commit = %q", r.Repo.Commit)
	}
	if r.Scorecard.Version != "v5.5.0" {
		t.Errorf("Scorecard.Version = %q, want v5.5.0", r.Scorecard.Version)
	}
	if r.Score != 8.9 {
		t.Errorf("Score = %v, want 8.9", r.Score)
	}
	if len(r.Checks) != 1 {
		t.Fatalf("len(Checks) = %d, want 1", len(r.Checks))
	}
	if r.Checks[0].Name != "Dependency-Update-Tool" || r.Checks[0].Score != 10 {
		t.Errorf("Checks[0] = %+v", r.Checks[0])
	}
	if r.Checks[0].Details != nil {
		t.Errorf("Checks[0].Details = %v, want nil", r.Checks[0].Details)
	}
	if r.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", r.Metadata)
	}
}

func TestParseWithMetadata(t *testing.T) {
	t.Parallel()

	r, err := Parse([]byte(json2WithMetadata))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if len(r.Metadata) != 1 || r.Metadata[0] != "example" {
		t.Errorf("Metadata = %v, want [example]", r.Metadata)
	}
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()

	if _, err := Parse([]byte("not json")); err == nil {
		t.Fatal("Parse() on invalid JSON = nil error, want error")
	}
}
