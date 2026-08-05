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

package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/uwu-tools/scorecard-api/internal/model"
)

const testCommit = "2418d6d95e928102e1f3f8d6e7b92f4f3c78631f"

func testRef(t *testing.T) model.RepoRef {
	t.Helper()
	ref, err := model.ParseRepoRef("github.com", "ossf", "scorecard")
	if err != nil {
		t.Fatalf("ParseRepoRef: %v", err)
	}
	return ref
}

// openStore opens a store at bucketURL and closes it at test end.
func openStore(t *testing.T, bucketURL string) *Store {
	t.Helper()
	s, err := Open(context.Background(), bucketURL)
	if err != nil {
		t.Fatalf("Open(%q): %v", bucketURL, err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestKey(t *testing.T) {
	t.Parallel()

	ref := testRef(t)
	if got, want := Key(ref, ""), "github.com/ossf/scorecard/results.json"; got != want {
		t.Errorf("Key(latest) = %q, want %q", got, want)
	}
	if got, want := Key(ref, testCommit), "github.com/ossf/scorecard/"+testCommit+"/results.json"; got != want {
		t.Errorf("Key(commit) = %q, want %q", got, want)
	}
}

func TestOpenEmptyURL(t *testing.T) {
	t.Parallel()

	if _, err := Open(context.Background(), ""); !errors.Is(err, errEmptyBucketURL) {
		t.Fatalf("Open(\"\") error = %v, want errEmptyBucketURL", err)
	}
}

func TestGetNotFound(t *testing.T) {
	t.Parallel()

	s := openStore(t, "mem://")
	if _, err := s.Get(context.Background(), testRef(t), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(miss) error = %v, want ErrNotFound", err)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	runRoundTrip(t, "mem://")
}

// runRoundTrip exercises the full Put/Get contract against a bucket URL. It is
// shared by the in-memory, fileblob, and S3-compatible backend tests.
func runRoundTrip(t *testing.T, bucketURL string) {
	t.Helper()

	ctx := context.Background()
	s := openStore(t, bucketURL)
	ref := testRef(t)
	latest := []byte(`{"score":8.9,"latest":true}`)
	pinned := []byte(`{"score":7.0,"commit":true}`)

	if err := s.Put(ctx, ref, "", latest); err != nil {
		t.Fatalf("Put(latest): %v", err)
	}
	if got, err := s.Get(ctx, ref, ""); err != nil || !bytes.Equal(got, latest) {
		t.Fatalf("Get(latest) = %q, %v; want %q", got, err, latest)
	}

	if err := s.Put(ctx, ref, testCommit, pinned); err != nil {
		t.Fatalf("Put(commit): %v", err)
	}
	if got, err := s.Get(ctx, ref, testCommit); err != nil || !bytes.Equal(got, pinned) {
		t.Fatalf("Get(commit) = %q, %v; want %q", got, err, pinned)
	}

	// The commit write must not have clobbered the independent latest pointer.
	if got, err := s.Get(ctx, ref, ""); err != nil || !bytes.Equal(got, latest) {
		t.Fatalf("Get(latest) after commit write = %q, %v; want %q", got, err, latest)
	}
}

func TestPutLatestAndCommit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t, "mem://")
	ref := testRef(t)
	body := []byte(`{"score":9.1}`)

	if err := s.PutLatestAndCommit(ctx, ref, testCommit, body); err != nil {
		t.Fatalf("PutLatestAndCommit: %v", err)
	}
	for _, commit := range []string{"", testCommit} {
		got, err := s.Get(ctx, ref, commit)
		if err != nil || !bytes.Equal(got, body) {
			t.Errorf("Get(%q) = %q, %v; want %q", commit, got, err, body)
		}
	}

	if err := s.PutLatestAndCommit(ctx, ref, "", body); !errors.Is(err, errEmptyCommit) {
		t.Errorf("PutLatestAndCommit(empty commit) error = %v, want errEmptyCommit", err)
	}
}

// TestRoundTripFileblob runs the contract against a local-filesystem bucket.
func TestRoundTripFileblob(t *testing.T) {
	t.Parallel()

	runRoundTrip(t, "file://"+t.TempDir())
}

// TestRoundTripS3 runs the contract against an S3-compatible bucket (e.g. MinIO).
// It is skipped unless SCORECARD_TEST_S3_URL is set to a gocloud.dev/blob s3://
// URL; credentials resolve via the AWS default chain.
func TestRoundTripS3(t *testing.T) {
	t.Parallel()

	bucketURL := os.Getenv("SCORECARD_TEST_S3_URL")
	if bucketURL == "" {
		t.Skip("set SCORECARD_TEST_S3_URL (e.g. a MinIO s3:// URL) to run the S3 integration test")
	}
	runRoundTrip(t, bucketURL)
}
