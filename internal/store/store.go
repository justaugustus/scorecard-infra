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

// Package store persists Scorecard results in a cloud-agnostic object store via
// gocloud.dev/blob. The backend (S3-compatible incl. MinIO, Azure Blob, GCS,
// local file, or in-memory) is selected entirely by the bucket URL, so nothing
// cloud-specific is compiled in (design D3).
//
// Object keys match ossf/scorecard-webapp exactly (design D4, confirmed in
// task 0.3):
//
//	{host}/{org}/{repo}/results.json            latest (mutable)
//	{host}/{org}/{repo}/{commit}/results.json   pinned (immutable)
//
// Bodies are canonical Scorecard JSON2.
package store

import (
	"context"
	"errors"
	"fmt"

	"gocloud.dev/blob"
	// Blank-import every backend driver so a bucket URL alone selects the
	// backend at runtime. Credentials resolve via each backend's default chain.
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"
)

// ErrNotFound is returned when no stored result exists for the requested key.
// The orchestrator treats it as a cache miss (design D2), not a fatal error.
var ErrNotFound = errors.New("store: result not found")

// errEmptyBucketURL is returned by Open when no bucket URL is configured.
var errEmptyBucketURL = errors.New("store: bucket URL is empty")

// Store is a cloud-agnostic Scorecard result store backed by gocloud.dev/blob.
type Store struct {
	bucket *blob.Bucket
}

// Open opens the bucket addressed by a gocloud.dev/blob URL, e.g.
// "file:///var/scorecard", "s3://my-bucket?region=us-east-1", or "mem://".
// It fails fast when the URL is empty or the backend cannot be opened.
func Open(ctx context.Context, bucketURL string) (*Store, error) {
	if bucketURL == "" {
		return nil, errEmptyBucketURL
	}
	bucket, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return nil, fmt.Errorf("store: opening bucket: %w", err)
	}
	return &Store{bucket: bucket}, nil
}

// Close releases the underlying bucket.
func (s *Store) Close() error {
	if err := s.bucket.Close(); err != nil {
		return fmt.Errorf("store: closing bucket: %w", err)
	}
	return nil
}

// TODO(group 3): key construction, Get/Put of JSON2 bodies, latest-pointer
// write-back on scan, and the not-found sentinel on miss.
