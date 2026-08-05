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

package model

import (
	"strings"
	"testing"
)

func TestParseRepoRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		host     string
		org      string
		repo     string
		want     RepoRef
		wantName string
		wantErr  bool
	}{
		{
			name:     "empty host defaults to github",
			org:      "ossf",
			repo:     "scorecard",
			want:     RepoRef{Host: "github.com", Org: "ossf", Repo: "scorecard"},
			wantName: "github.com/ossf/scorecard",
		},
		{
			name: "github explicit",
			host: "github.com",
			org:  "ossf",
			repo: "scorecard",
			want: RepoRef{Host: "github.com", Org: "ossf", Repo: "scorecard"},
		},
		{
			name: "gitlab accepted",
			host: "gitlab.com",
			org:  "gitlab-org",
			repo: "gitlab",
			want: RepoRef{Host: "gitlab.com", Org: "gitlab-org", Repo: "gitlab"},
		},
		{
			name: "host is lowercased",
			host: "GitHub.com",
			org:  "ossf",
			repo: "scorecard",
			want: RepoRef{Host: "github.com", Org: "ossf", Repo: "scorecard"},
		},
		{name: "unsupported host", host: "bitbucket.org", org: "a", repo: "b", wantErr: true},
		{name: "empty org", org: "", repo: "b", wantErr: true},
		{name: "empty repo", org: "a", repo: "", wantErr: true},
		{name: "slash in repo", org: "a", repo: "b/c", wantErr: true},
		{name: "backslash in org", org: `a\b`, repo: "c", wantErr: true},
		{name: "dotdot org rejected", org: "..", repo: "b", wantErr: true},
		{name: "dot repo rejected", org: "a", repo: ".", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseRepoRef(tt.host, tt.org, tt.repo)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRepoRef(%q, %q, %q) = %+v, want error", tt.host, tt.org, tt.repo, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepoRef(%q, %q, %q) unexpected error: %v", tt.host, tt.org, tt.repo, err)
			}
			if got != tt.want {
				t.Errorf("ParseRepoRef(%q, %q, %q) = %+v, want %+v", tt.host, tt.org, tt.repo, got, tt.want)
			}
			if tt.wantName != "" && got.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", got.Name(), tt.wantName)
			}
		})
	}
}

func TestParseCommit(t *testing.T) {
	t.Parallel()

	const valid = "abcdef0123456789abcdef0123456789abcdef01"

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty is latest", in: "", want: ""},
		{name: "valid lowercase", in: valid, want: valid},
		{name: "uppercase normalized to lowercase", in: strings.ToUpper(valid), want: valid},
		{name: "too short", in: "abc123", wantErr: true},
		{name: "too long", in: valid + "0", wantErr: true},
		{name: "non-hex characters", in: strings.Repeat("z", 40), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseCommit(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseCommit(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCommit(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseCommit(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
