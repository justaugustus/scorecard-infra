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

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Supported SCM hosts. An empty host defaults to DefaultHost.
const (
	HostGitHub  = "github.com"
	HostGitLab  = "gitlab.com"
	DefaultHost = HostGitHub
)

var (
	errUnsupportedHost = errors.New("model: unsupported host")
	errInvalidSegment  = errors.New("model: invalid path segment")
	errInvalidCommit   = errors.New("model: commit must be a 40-character hex SHA")
)

// commitRE matches a full 40-character Git SHA-1 commit hash.
var commitRE = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// RepoRef identifies a repository the server serves or scans, as parsed from a
// platform/owner/repo request path.
type RepoRef struct {
	Host string
	Org  string
	Repo string
}

// ParseRepoRef validates and normalizes a platform/owner/repo reference. An
// empty host defaults to github.com; github.com and gitlab.com are supported.
// The org and repo segments must be non-empty and safe to use as object-key
// path components (no separators, no "." or "..", no control characters).
func ParseRepoRef(host, org, repo string) (RepoRef, error) {
	if host == "" {
		host = DefaultHost
	}
	host = strings.ToLower(host)
	switch host {
	case HostGitHub, HostGitLab:
	default:
		return RepoRef{}, fmt.Errorf("%w: %q (supported: %s, %s)", errUnsupportedHost, host, HostGitHub, HostGitLab)
	}
	if err := validateSegment("org", org); err != nil {
		return RepoRef{}, err
	}
	if err := validateSegment("repo", repo); err != nil {
		return RepoRef{}, err
	}
	return RepoRef{Host: host, Org: org, Repo: repo}, nil
}

// Name returns the canonical repository name, host/org/repo, as used in the
// JSON2 repo.name field and as the object-key prefix.
func (r RepoRef) Name() string {
	return r.Host + "/" + r.Org + "/" + r.Repo
}

// ParseCommit validates an optional commit SHA. An empty string denotes the
// latest result; a non-empty value must be a 40-character hex SHA and is
// normalized to lowercase.
func ParseCommit(sha string) (string, error) {
	if sha == "" {
		return "", nil
	}
	if !commitRE.MatchString(sha) {
		return "", fmt.Errorf("%w: got %q", errInvalidCommit, sha)
	}
	return strings.ToLower(sha), nil
}

// validateSegment rejects path segments that are empty, relative (. or ..),
// contain a path separator, or contain control characters — any of which would
// be unsafe when composed into an object key.
func validateSegment(kind, s string) error {
	switch {
	case s == "":
		return fmt.Errorf("%w: %s is empty", errInvalidSegment, kind)
	case s == "." || s == "..":
		return fmt.Errorf("%w: %s must not be %q", errInvalidSegment, kind, s)
	case strings.ContainsAny(s, `/\`):
		return fmt.Errorf("%w: %s must not contain a path separator", errInvalidSegment, kind)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: %s must not contain control characters", errInvalidSegment, kind)
		}
	}
	return nil
}
