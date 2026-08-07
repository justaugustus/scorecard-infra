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

	"github.com/uwu-tools/scorecard-infra/internal/fallback"
	"github.com/uwu-tools/scorecard-infra/internal/model"
	"github.com/uwu-tools/scorecard-infra/internal/scan"
	"github.com/uwu-tools/scorecard-infra/internal/store"
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

// fakeFallback is a controllable Fallback for tests.
type fakeFallback struct {
	err    error
	result *scan.Result
	mu     sync.Mutex
	calls  int
}

func (f *fakeFallback) Fetch(_ context.Context, _ model.RepoRef) (*scan.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.result, f.err
}

func (f *fakeFallback) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeFlags is a static FlagSource for tests.
type fakeFlags struct {
	mode    string
	enabled bool
}

func (f fakeFlags) Bool(_ context.Context, _ string, _ bool) bool { return f.enabled }

func (f fakeFlags) String(_ context.Context, _, def string) string {
	if f.mode == "" {
		return def
	}
	return f.mode
}

// newOrchOpts is newOrch with extra options (e.g. a fallback or flag source).
func newOrchOpts(t *testing.T, sc scan.Scanner, cfg Config, opts ...Option) *Orchestrator {
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
	all := append([]Option{WithClock(func() time.Time { return fixedNow })}, opts...)
	return New(st, sc, cfg, all...)
}

// noScan fails the test if the scanner is called; used where the fallback or a
// cache hit must serve without scanning.
func noScan(t *testing.T) *scan.FakeScanner {
	t.Helper()
	return &scan.FakeScanner{ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
		t.Error("scanner called unexpectedly")
		return nil, errors.New("unexpected scan")
	}}
}

func liveScanner(commit string) *scan.FakeScanner {
	return &scan.FakeScanner{ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
		return &scan.Result{JSON2: json2(fixedNow, commit, 8.0), ResolvedCommit: commit, Complete: true}, nil
	}}
}

func TestFetchFirstServesAndBackfills(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{result: &scan.Result{
		JSON2: json2(fixedNow, "up-sha", 7.5), ResolvedCommit: "up-sha", Complete: true,
	}}
	fake := noScan(t) // fetch-first hit must not scan
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second}, WithFallback(fb))
	ref := testRef(t)

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceUpstream || out.Provenance.Commit != "up-sha" {
		t.Errorf("outcome = %+v, want upstream up-sha", out.Provenance)
	}
	if fb.Calls() != 1 {
		t.Errorf("fallback called %d times, want 1", fb.Calls())
	}
	// The upstream result was backfilled tagged upstream.
	_, origin, gerr := o.store.GetWithOrigin(context.Background(), ref, "")
	if gerr != nil || origin != store.OriginUpstream {
		t.Errorf("backfill origin = %q, %v; want upstream", origin, gerr)
	}
}

func TestFetchFirstMissThenScans(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{err: fallback.ErrFallbackMiss}
	fake := liveScanner("live-sha")
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second}, WithFallback(fb))

	out, err := o.GetOrProduce(context.Background(), testRef(t), "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceLive {
		t.Errorf("source = %q, want live after an upstream miss", out.Provenance.Source)
	}
	if fb.Calls() != 1 || fake.Calls() != 1 {
		t.Errorf("fallback=%d scanner=%d, want 1 and 1", fb.Calls(), fake.Calls())
	}
}

func TestSafetyNetScanSuccessSkipsUpstream(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{result: &scan.Result{JSON2: json2(fixedNow, "up-sha", 7.5), ResolvedCommit: "up-sha"}}
	fake := liveScanner("live-sha")
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second},
		WithFallback(fb), WithFlags(fakeFlags{enabled: true, mode: ModeSafetyNet}))

	out, err := o.GetOrProduce(context.Background(), testRef(t), "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceLive {
		t.Errorf("source = %q, want live", out.Provenance.Source)
	}
	if fb.Calls() != 0 {
		t.Errorf("fallback called %d times on a successful scan, want 0", fb.Calls())
	}
}

func TestSafetyNetSkipRescuedByUpstream(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{result: &scan.Result{
		JSON2: json2(fixedNow, "up-sha", 7.5), ResolvedCommit: "up-sha", Complete: true,
	}}
	fake := &scan.FakeScanner{ScanFunc: func(_ context.Context, _ model.RepoRef, _ string) (*scan.Result, error) {
		return nil, scan.ErrSkipped
	}}
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second},
		WithFallback(fb), WithFlags(fakeFlags{enabled: true, mode: ModeSafetyNet}))

	out, err := o.GetOrProduce(context.Background(), testRef(t), "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceUpstream {
		t.Errorf("source = %q, want upstream (rescued a skipped scan)", out.Provenance.Source)
	}
	if fb.Calls() != 1 {
		t.Errorf("fallback called %d times, want 1", fb.Calls())
	}
}

func TestCommitPinnedBypassesFallback(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{result: &scan.Result{JSON2: json2(fixedNow, "up-sha", 7.5), ResolvedCommit: "up-sha"}}
	const commit = "2418d6d95e928102e1f3f8d6e7b92f4f3c78631f"
	fake := liveScanner(commit)
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second}, WithFallback(fb))

	out, err := o.GetOrProduce(context.Background(), testRef(t), commit)
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceLive {
		t.Errorf("source = %q, want live (commit-pinned scans)", out.Provenance.Source)
	}
	if fb.Calls() != 0 {
		t.Errorf("fallback called %d times for a commit-pinned request, want 0", fb.Calls())
	}
}

func TestKillSwitchDisablesFallback(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{result: &scan.Result{JSON2: json2(fixedNow, "up-sha", 7.5), ResolvedCommit: "up-sha"}}
	fake := liveScanner("live-sha")
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, SyncTimeout: time.Second},
		WithFallback(fb), WithFlags(fakeFlags{enabled: false}))

	out, err := o.GetOrProduce(context.Background(), testRef(t), "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceLive {
		t.Errorf("source = %q, want live (fallback disabled)", out.Provenance.Source)
	}
	if fb.Calls() != 0 {
		t.Errorf("fallback called %d times while disabled, want 0", fb.Calls())
	}
}

func TestUpstreamEntryFreshByMaxAge(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{}
	fake := noScan(t)
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, FallbackMaxAge: 24 * time.Hour, SyncTimeout: time.Second},
		WithFallback(fb))
	ref := testRef(t)
	// An upstream entry two hours old: stale for a 1h local TTL, but fresh under
	// the 24h fallback max-age, so it is served without a fetch or scan.
	if err := o.store.PutWithOrigin(context.Background(), ref, "",
		json2(fixedNow.Add(-2*time.Hour), "up-sha", 7.0), store.OriginUpstream); err != nil {
		t.Fatalf("seed upstream: %v", err)
	}

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Source != model.SourceUpstream {
		t.Errorf("source = %q, want upstream", out.Provenance.Source)
	}
	if fb.Calls() != 0 {
		t.Errorf("fallback called %d times for a fresh upstream entry, want 0", fb.Calls())
	}
}

func TestUpstreamEntryStaleByMaxAge(t *testing.T) {
	t.Parallel()

	fb := &fakeFallback{result: &scan.Result{
		JSON2: json2(fixedNow, "new-up-sha", 8.0), ResolvedCommit: "new-up-sha", Complete: true,
	}}
	fake := noScan(t)
	o := newOrchOpts(t, fake, Config{TTL: time.Hour, FallbackMaxAge: 24 * time.Hour, SyncTimeout: time.Second},
		WithFallback(fb))
	ref := testRef(t)
	// An upstream entry 48h old exceeds the 24h max-age, so it is refreshed
	// (here, re-fetched from the upstream in fetch-first).
	if err := o.store.PutWithOrigin(context.Background(), ref, "",
		json2(fixedNow.Add(-48*time.Hour), "old-up-sha", 5.0), store.OriginUpstream); err != nil {
		t.Fatalf("seed upstream: %v", err)
	}

	out, err := o.GetOrProduce(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("GetOrProduce: %v", err)
	}
	if out.Provenance.Commit != "new-up-sha" || fb.Calls() != 1 {
		t.Errorf("outcome=%+v fallback=%d; want refreshed new-up-sha via 1 fetch", out.Provenance, fb.Calls())
	}
}
