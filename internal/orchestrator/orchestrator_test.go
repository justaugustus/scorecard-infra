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

package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/uwu-tools/scorecard-api/internal/model"
	"github.com/uwu-tools/scorecard-api/internal/scan"
	"github.com/uwu-tools/scorecard-api/internal/store"
)

// fixedNow is the clock all tests run against.
var fixedNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func testRef(t *testing.T) model.RepoRef {
	t.Helper()
	ref, err := model.ParseRepoRef("github.com", "ossf", "scorecard")
	if err != nil {
		t.Fatalf("ParseRepoRef: %v", err)
	}
	return ref
}

// json2 builds a minimal canonical JSON2 body with the given date, commit, and
// aggregate score.
func json2(date time.Time, commit string, score float64) []byte {
	return fmt.Appendf(nil,
		`{"date":%q,"repo":{"name":"github.com/ossf/scorecard","commit":%q},`+
			`"scorecard":{"version":"v5.5.0","commit":"engine-sha"},"score":%.1f,"checks":[]}`,
		date.Format(time.RFC3339), commit, score)
}

func newOrch(t *testing.T, sc scan.Scanner, cfg Config) *Orchestrator {
	t.Helper()
	st, err := store.Open(context.Background(), "mem://")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if cerr := st.Close(); cerr != nil {
			t.Errorf("store.Close: %v", cerr)
		}
	})
	return New(st, sc, cfg, WithClock(func() time.Time { return fixedNow }))
}

// seed writes a body directly to the store, bypassing the orchestrator.
func seed(t *testing.T, o *Orchestrator, ref model.RepoRef, commit string, body []byte) {
	t.Helper()
	if err := o.store.Put(context.Background(), ref, commit, body); err != nil {
		t.Fatalf("seed Put: %v", err)
	}
}

func TestHitFresh(t *testing.T) {
	t.Parallel()

	fake := &scan.FakeScanner{}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second})
	ref := testRef(t)
	seed(t, o, ref, "", json2(fixedNow, "cached-sha", 9.0))

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if !out.Ready || out.Provenance.Source != model.SourceCached {
		t.Errorf("outcome = %+v, want ready cached", out)
	}
	if out.Provenance.Commit != "cached-sha" {
		t.Errorf("Provenance.Commit = %q, want cached-sha", out.Provenance.Commit)
	}
	if fake.Calls() != 0 {
		t.Errorf("scanner called %d times on a fresh hit, want 0", fake.Calls())
	}
}

func TestMissScans(t *testing.T) {
	t.Parallel()

	fake := &scan.FakeScanner{
		ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
			return &scan.Result{JSON2: json2(fixedNow, "live-sha", 8.0), ResolvedCommit: "live-sha", Complete: true}, nil
		},
	}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second})
	ref := testRef(t)

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if !out.Ready || out.Provenance.Source != model.SourceLive {
		t.Errorf("outcome = %+v, want ready live", out)
	}
	if fake.Calls() != 1 {
		t.Errorf("scanner called %d times, want 1", fake.Calls())
	}
	// The result must have been persisted to the latest key.
	if _, gerr := o.store.Get(context.Background(), ref, ""); gerr != nil {
		t.Errorf("latest not persisted after scan: %v", gerr)
	}
}

func TestStaleRefresh(t *testing.T) {
	t.Parallel()

	fake := &scan.FakeScanner{
		ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
			return &scan.Result{JSON2: json2(fixedNow, "fresh-sha", 8.0), ResolvedCommit: "fresh-sha", Complete: true}, nil
		},
	}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second})
	ref := testRef(t)
	// A latest result from two hours ago is stale against a one-hour TTL.
	seed(t, o, ref, "", json2(fixedNow.Add(-2*time.Hour), "old-sha", 5.0))

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceLive || out.Provenance.Commit != "fresh-sha" {
		t.Errorf("stale latest not refreshed: %+v", out.Provenance)
	}
	if fake.Calls() != 1 {
		t.Errorf("scanner called %d times, want 1", fake.Calls())
	}
}

func TestCommitPinnedImmutable(t *testing.T) {
	t.Parallel()

	fake := &scan.FakeScanner{}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second})
	ref := testRef(t)
	const commit = "2418d6d95e928102e1f3f8d6e7b92f4f3c78631f"
	// Even a very old commit-pinned result is served without rescanning.
	seed(t, o, ref, commit, json2(fixedNow.Add(-1000*time.Hour), commit, 7.0))

	out, err := o.GetOrProduce(context.Background(), ref, commit)
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceCached {
		t.Errorf("commit-pinned served as %q, want cached", out.Provenance.Source)
	}
	if fake.Calls() != 0 {
		t.Errorf("scanner called %d times for an immutable commit, want 0", fake.Calls())
	}
}

func TestConcurrentCoalescing(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	fake := &scan.FakeScanner{
		ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
			once.Do(func() { close(started) })
			<-release
			return &scan.Result{JSON2: json2(fixedNow, "one-sha", 8.0), ResolvedCommit: "one-sha", Complete: true}, nil
		},
	}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: 5 * time.Second})
	ref := testRef(t)

	const n = 8
	outs := make([]*Outcome, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outs[i], errs[i] = o.GetOrProduce(context.Background(), ref, "")
		}(i)
	}

	<-started                         // one scan is in flight
	time.Sleep(20 * time.Millisecond) // let the rest subscribe to the flight
	close(release)
	wg.Wait()

	for i := range n {
		if errs[i] != nil || outs[i] == nil || !outs[i].Ready {
			t.Fatalf("request %d = %+v, %v; want a ready outcome", i, outs[i], errs[i])
		}
	}
	if fake.Calls() != 1 {
		t.Errorf("scanner called %d times for %d concurrent identical requests, want 1", fake.Calls(), n)
	}
}

func TestTimeoutReturns202(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	fake := &scan.FakeScanner{
		ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
			<-release // block past the sync timeout
			return &scan.Result{JSON2: json2(fixedNow, "slow-sha", 8.0), ResolvedCommit: "slow-sha", Complete: true}, nil
		},
	}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: 20 * time.Millisecond, ScanTimeout: 5 * time.Second})
	ref := testRef(t)

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Ready {
		t.Fatalf("outcome = %+v, want not-ready (202)", out)
	}
	if out.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want > 0", out.RetryAfter)
	}

	// Let the background scan finish and confirm it populated the cache.
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, gerr := o.store.Get(context.Background(), ref, ""); gerr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background scan did not populate the cache")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSkipPropagates(t *testing.T) {
	t.Parallel()

	fake := &scan.FakeScanner{
		ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
			return nil, scan.ErrSkipped
		},
	}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second})

	out, err := o.GetOrProduce(context.Background(), testRef(t), "")
	if !errors.Is(err, scan.ErrSkipped) {
		t.Fatalf("GetOrProduce error = %v, want scan.ErrSkipped", err)
	}
	if out != nil {
		t.Errorf("outcome = %+v, want nil on skip", out)
	}
}

func TestInconclusiveScorePreserved(t *testing.T) {
	t.Parallel()

	fake := &scan.FakeScanner{}
	o := newOrch(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second})
	ref := testRef(t)
	body := []byte(`{"date":"2026-08-05T12:00:00Z","repo":{"name":"github.com/ossf/scorecard","commit":"c"},` +
		`"scorecard":{"version":"v5.5.0","commit":"e"},"score":-1,"checks":[{"name":"X","score":-1,` +
		`"reason":"inconclusive","details":null,"documentation":{"short":"s","url":"u"}}]}`)
	seed(t, o, ref, "", body)

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	// The body is served verbatim (score -1 preserved, not converted).
	if !bytes.Contains(out.Body, []byte(`"score":-1`)) {
		t.Errorf("body did not preserve -1 score: %s", out.Body)
	}
	if out.Provenance.Complete {
		t.Error("Provenance.Complete = true, want false for an inconclusive check")
	}
}
