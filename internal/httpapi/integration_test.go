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

// Package httpapi_test contains an offline end-to-end integration test: the real
// HTTP handlers over the real orchestrator over a real fileblob store, wired as a
// consumer would wire them. It uses a fake scanner (a cache HIT never scans, and a
// MISS is served a canned result), so it needs no network, SCM token, or Docker.
// The live-engine acceptance against scorecard-mcp is group 8.
package httpapi_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uwu-tools/scorecard-infra/internal/httpapi"
	"github.com/uwu-tools/scorecard-infra/internal/model"
	"github.com/uwu-tools/scorecard-infra/internal/orchestrator"
	"github.com/uwu-tools/scorecard-infra/internal/scan"
	"github.com/uwu-tools/scorecard-infra/internal/store"
)

// response is the part of an HTTP response the tests assert on. Returning this
// (rather than *http.Response) keeps body handling inside get.
type response struct {
	header http.Header
	body   string
	status int
}

// setup wires a real fileblob-backed server and returns its store, base URL, and
// the on-disk bucket directory.
func setup(t *testing.T, scanner scan.Scanner) (*store.Store, string, string) {
	t.Helper()

	dir := t.TempDir()
	st, err := store.Open(context.Background(), "file://"+dir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if cerr := st.Close(); cerr != nil {
			t.Errorf("store.Close: %v", cerr)
		}
	})

	orch := orchestrator.New(st, scanner, orchestrator.Config{
		TTL:         time.Hour,
		SyncTimeout: 5 * time.Second,
		ScanTimeout: 30 * time.Second,
		Concurrency: 2,
	})
	ts := httptest.NewServer(httpapi.New(orch, httpapi.DefaultCapabilities(time.Hour)).Handler())
	t.Cleanup(ts.Close)

	return st, ts.URL, dir
}

func testRef(t *testing.T) model.RepoRef {
	t.Helper()
	ref, err := model.ParseRepoRef("github.com", "ossf", "scorecard")
	if err != nil {
		t.Fatalf("ParseRepoRef: %v", err)
	}
	return ref
}

// json2 builds a minimal, fresh JSON2 body.
func json2(commit string, score float64) []byte {
	return fmt.Appendf(nil,
		`{"date":%q,"repo":{"name":"github.com/ossf/scorecard","commit":%q},`+
			`"scorecard":{"version":"v5.5.0","commit":"engine-sha"},"score":%.1f,"checks":[]}`,
		time.Now().UTC().Format(time.RFC3339), commit, score)
}

// get performs a context-bearing GET, fully consuming and closing the body.
func get(t *testing.T, url string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("close body: %v", cerr)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func TestIntegrationCacheHit(t *testing.T) {
	t.Parallel()

	// A HIT must never scan.
	fake := &scan.FakeScanner{
		ScanFunc: func(context.Context, model.RepoRef, string) (*scan.Result, error) {
			t.Error("scanner must not run on a cache hit")
			return nil, errors.New("unexpected scan")
		},
	}
	st, base, dir := setup(t, fake)
	if err := st.Put(context.Background(), testRef(t), "", json2("cached-sha", 8.9)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := get(t, base+"/projects/github.com/ossf/scorecard")
	if r.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", r.status)
	}
	if src := r.header.Get("X-Scorecard-Source"); src != "cached" {
		t.Errorf("X-Scorecard-Source = %q, want cached", src)
	}
	if c := r.header.Get("X-Scorecard-Resolved-Commit"); c != "cached-sha" {
		t.Errorf("resolved-commit header = %q, want cached-sha", c)
	}
	if !strings.Contains(r.body, `"score":8.9`) {
		t.Errorf("body = %s, want the seeded JSON2", r.body)
	}
	// The object lives on disk at the webapp key path.
	if _, err := os.Stat(filepath.Join(dir, "github.com", "ossf", "scorecard", "results.json")); err != nil {
		t.Errorf("expected result file at the latest key: %v", err)
	}
}

func TestIntegrationMissThenHit(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	fake := &scan.FakeScanner{
		ScanFunc: func(context.Context, model.RepoRef, string) (*scan.Result, error) {
			calls.Add(1)
			return &scan.Result{JSON2: json2("live-sha", 7.5), ResolvedCommit: "live-sha", Complete: true}, nil
		},
	}
	_, base, dir := setup(t, fake)

	// First request: MISS -> live scan -> persist -> serve.
	first := get(t, base+"/projects/github.com/ossf/scorecard")
	if first.status != http.StatusOK || first.header.Get("X-Scorecard-Source") != "live" {
		t.Fatalf("first request: status=%d source=%q, want 200 live", first.status, first.header.Get("X-Scorecard-Source"))
	}
	if !strings.Contains(first.body, `"score":7.5`) {
		t.Errorf("body = %s, want the scanned JSON2", first.body)
	}

	// Second request: HIT from the freshly persisted result, no new scan.
	second := get(t, base+"/projects/github.com/ossf/scorecard")
	if second.header.Get("X-Scorecard-Source") != "cached" {
		t.Errorf("second request source = %q, want cached", second.header.Get("X-Scorecard-Source"))
	}
	if calls.Load() != 1 {
		t.Errorf("scanner called %d times, want 1", calls.Load())
	}

	// A latest scan populates both the latest pointer and the commit key.
	for _, key := range []string{
		filepath.Join(dir, "github.com", "ossf", "scorecard", "results.json"),
		filepath.Join(dir, "github.com", "ossf", "scorecard", "live-sha", "results.json"),
	} {
		if _, err := os.Stat(key); err != nil {
			t.Errorf("expected object at %s: %v", key, err)
		}
	}
}

func TestIntegrationBadgeCapabilitiesHealth(t *testing.T) {
	t.Parallel()

	st, base, _ := setup(t, &scan.FakeScanner{})
	if err := st.Put(context.Background(), testRef(t), "", json2("c", 8.9)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	badge := get(t, base+"/projects/github.com/ossf/scorecard/badge")
	if badge.status != http.StatusOK || !strings.HasPrefix(badge.header.Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("badge: status=%d ct=%q", badge.status, badge.header.Get("Content-Type"))
	}
	if !strings.Contains(badge.body, "<svg") || !strings.Contains(badge.body, "8.9") {
		t.Errorf("badge body missing content: %s", badge.body)
	}

	caps := get(t, base+"/capabilities")
	if caps.status != http.StatusOK || !strings.Contains(caps.body, "cached+live") {
		t.Errorf("capabilities: status=%d body=%s", caps.status, caps.body)
	}

	if health := get(t, base+"/health"); health.status != http.StatusOK {
		t.Errorf("/health status = %d, want 200", health.status)
	}
}
