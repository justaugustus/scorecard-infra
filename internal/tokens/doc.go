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

// Package tokens manages SCM credentials and rate limiting for live scans
// (design D8). It provides a token pool (GitHub App installation tokens or a PAT
// pool), a per-host rate limiter, and backoff/retry. SCM API rate limits are the
// real scaling bottleneck, and a single token is unsafe across concurrent scans.
//
// TODO(group 4): implement the token pool, per-host limiter, and backoff/retry.
package tokens
