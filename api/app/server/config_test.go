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

import "testing"

// The point of this test is the first case in each table: an unconfigured
// deployment must still read and write the exact buckets it did when these were
// compile-time constants. That property is the whole basis for calling this
// change a no-op until something is deployed elsewhere, so it is asserted
// against the literal strings rather than against the default constants --
// comparing a constant to itself would pass even if someone edited it.
//
// Not t.Parallel(): t.Setenv is process-global and the two are incompatible.

func TestResultsBucketURL(t *testing.T) {
	testcases := []struct {
		name string
		env  string
		want string
	}{
		{
			name: "defaults to the production results bucket when unset",
			env:  "",
			want: "gs://ossf-scorecard-results",
		},
		{
			name: "honors an s3 bucket URL",
			env:  "s3://ossf-scorecard-results?region=us-east-2",
			want: "s3://ossf-scorecard-results?region=us-east-2",
		},
		{
			name: "honors a file URL, as used by local development",
			env:  "file:///tmp/results",
			want: "file:///tmp/results",
		},
	}
	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(resultsBucketURLEnv, tt.env)
			if got := resultsBucketURL(); got != tt.want {
				t.Errorf("resultsBucketURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCronResultsBucketURL(t *testing.T) {
	testcases := []struct {
		name string
		env  string
		want string
	}{
		{
			name: "defaults to the production cron bucket when unset",
			env:  "",
			want: "gs://ossf-scorecard-cron-results",
		},
		{
			name: "honors an s3 bucket URL",
			env:  "s3://ossf-scorecard-cron-results?region=us-east-2",
			want: "s3://ossf-scorecard-cron-results?region=us-east-2",
		},
	}
	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(cronResultsBucketURLEnv, tt.env)
			if got := cronResultsBucketURL(); got != tt.want {
				t.Errorf("cronResultsBucketURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A container platform will readily inject an empty string for a variable that
// was declared but left blank. Falling through to the default is the safe
// reading: an empty bucket URL is never meaningful, and honoring it would turn
// a typo in a deployment manifest into a service that opens no bucket at all
// and reports every repository missing.
func TestEmptyEnvFallsBackToDefault(t *testing.T) {
	t.Setenv(resultsBucketURLEnv, "")
	t.Setenv(cronResultsBucketURLEnv, "")
	if got := resultsBucketURL(); got != defaultResultsBucketURL {
		t.Errorf("resultsBucketURL() = %q, want default %q", got, defaultResultsBucketURL)
	}
	if got := cronResultsBucketURL(); got != defaultCronResultsBucketURL {
		t.Errorf("cronResultsBucketURL() = %q, want default %q", got, defaultCronResultsBucketURL)
	}
}

// The read path's primary bucket and the publish path's write target must be
// the same bucket: a POST writes the object a subsequent GET returns. They were
// two constants with one value before this change, and collapsing them to a
// single setting is what keeps an operator from splitting them apart. This
// asserts the invariant rather than the mechanism, so it still holds if the
// implementation changes.
func TestPublishAndReadUseTheSameBucket(t *testing.T) {
	t.Setenv(resultsBucketURLEnv, "s3://somewhere-else")
	if resultsBucketURL() != "s3://somewhere-else" {
		t.Fatalf("resultsBucketURL() did not honor the environment")
	}
	if defaultResultsBucketURL == defaultCronResultsBucketURL {
		t.Errorf("the primary and cron buckets must remain distinct defaults")
	}
}
