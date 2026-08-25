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
	"os"

	// Blank-import every blob backend a deployment may address. The scheme in a
	// bucket URL selects the driver at run time, so an unlinked driver fails at
	// blob.OpenBucket -- at the first request, in production -- rather than at
	// compile time. gcsblob is what this service has always used; s3blob is what
	// makes an s3:// bucket URL work without a further code change.
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

// Environment variables selecting the buckets this service reads and writes.
const (
	resultsBucketURLEnv     = "SCORECARD_RESULTS_BUCKET_URL"
	cronResultsBucketURLEnv = "SCORECARD_CRON_RESULTS_BUCKET_URL"
)

// Defaults matching the buckets the service reads and writes in production
// today. Keeping them as the fallback is what makes configurability a no-op:
// an existing deployment that sets neither variable behaves exactly as it did
// when these were compile-time constants.
const (
	defaultResultsBucketURL     = "gs://ossf-scorecard-results"
	defaultCronResultsBucketURL = "gs://ossf-scorecard-cron-results"
)

// resultsBucketURL is the primary results bucket: the one the publish path
// writes and the read path checks first.
//
// Deliberately one setting rather than two. These were separate constants with
// identical values in get_results.go and post_results.go, and the equality was
// load-bearing -- a POST writes the object a subsequent GET returns. Exposing
// them as two variables would let an operator set them to different buckets and
// get a service that accepts uploads and then reports them missing, with
// nothing failing loudly enough to notice.
func resultsBucketURL() string {
	return envOr(resultsBucketURLEnv, defaultResultsBucketURL)
}

// cronResultsBucketURL is the fallback the read path tries when a repository
// has no Action-published result: the bucket the weekly scan writes. It is a
// genuinely different bucket, written by a different system, so it stays a
// separate setting.
func cronResultsBucketURL() string {
	return envOr(cronResultsBucketURLEnv, defaultCronResultsBucketURL)
}

// envOr treats an empty value as unset. Container platforms readily inject an
// empty string for a variable someone declared but left blank, and an empty
// bucket URL is never a meaningful configuration -- it would only turn a
// misconfiguration into an unreadable "open bucket" error at request time.
func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
