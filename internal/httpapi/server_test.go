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

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ossf/scorecard-infra/internal/model"
	"github.com/ossf/scorecard-infra/internal/orchestrator"
	"github.com/ossf/scorecard-infra/internal/scan"
)

// fakeProducer returns preset outcomes/errors so handler behavior is tested
// without the orchestrator's internals (covered by its own package tests).
type fakeProducer struct {
	out *orchestrator.Outcome
	err error
}

func (f *fakeProducer) GetOrProduce(_ context.Context, _ model.RepoRef, _ string) (*orchestrator.Outcome, error) {
	return f.out, f.err
}

func json2(score float64) []byte {
	return fmt.Appendf(nil,
		`{"date":"2026-08-05T12:00:00Z","repo":{"name":"github.com/ossf/scorecard","commit":"c"},`+
			`"scorecard":{"version":"v5.5.0","commit":"e"},"score":%.1f,"checks":[]}`, score)
}

func liveOutcome(score float64) *orchestrator.Outcome {
	return &orchestrator.Outcome{
		Ready: true,
		Body:  json2(score),
		Provenance: model.Provenance{
			Source: model.SourceLive, Commit: "c", Date: "2026-08-05T12:00:00Z",
			ScorecardVersion: "v5.5.0", Complete: true,
		},
	}
}

func do(t *testing.T, srv *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestResultReady(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{out: liveOutcome(8.9)}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != contentTypeJSON {
		t.Errorf("Content-Type = %q, want %q", ct, contentTypeJSON)
	}
	if src := rec.Header().Get(headerSource); src != "live" {
		t.Errorf("%s = %q, want live", headerSource, src)
	}
	if c := rec.Header().Get(headerComplete); c != "true" {
		t.Errorf("%s = %q, want true", headerComplete, c)
	}
	if !strings.Contains(rec.Body.String(), `"score":8.9`) {
		t.Errorf("body = %s, want the JSON2 result", rec.Body.String())
	}
}

func TestResultMalformedRef(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{out: liveOutcome(8.0)}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/bitbucket.org/a/b")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for unsupported host", rec.Code)
	}
}

func TestResultBadCommit(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{out: liveOutcome(8.0)}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard?commit=nothex")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a bad commit", rec.Code)
	}
}

func TestResultSkipIs404(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{err: scan.ErrSkipped}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a skipped repo", rec.Code)
	}
}

func TestResultScanFailureIs502(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{err: errors.New("scan blew up")}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 for a scan failure", rec.Code)
	}
}

func TestResultNotReadyIs202(t *testing.T) {
	t.Parallel()

	out := &orchestrator.Outcome{Ready: false, RetryAfter: 10 * time.Second}
	srv := New(&fakeProducer{out: out}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "10" {
		t.Errorf("Retry-After = %q, want 10", ra)
	}
}

func TestBadge(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{out: liveOutcome(8.9)}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard/badge")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<svg") || !strings.Contains(body, badgeLabel) || !strings.Contains(body, "8.9") {
		t.Errorf("badge body missing expected content: %s", body)
	}
}

func TestBadgeInconclusive(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{out: liveOutcome(-1)}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard/badge")
	if !strings.Contains(rec.Body.String(), "inconclusive") {
		t.Errorf("badge for -1 should read inconclusive: %s", rec.Body.String())
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{}, DefaultCapabilities(30*time.Minute))
	rec := do(t, srv, http.MethodGet, "/capabilities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var caps Capabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decoding capabilities: %v", err)
	}
	if caps.Mode != "cached+live" || caps.Checks != "all" || caps.RequiresOptIn {
		t.Errorf("caps = %+v, want cached+live/all/no-opt-in", caps)
	}
	if caps.LatestTTLSeconds != 1800 {
		t.Errorf("LatestTTLSeconds = %d, want 1800", caps.LatestTTLSeconds)
	}
	if len(caps.Caveats) == 0 {
		t.Error("caveats should not be empty")
	}
}

func TestCapabilitiesWithFallback(t *testing.T) {
	t.Parallel()

	base := DefaultCapabilities(time.Hour)
	srv := New(&fakeProducer{}, base.WithFallback())
	rec := do(t, srv, http.MethodGet, "/capabilities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var caps Capabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decoding capabilities: %v", err)
	}
	if caps.Mode != "cached+upstream+live" {
		t.Errorf("Mode = %q, want cached+upstream+live", caps.Mode)
	}
	if len(caps.Caveats) <= len(base.Caveats) {
		t.Errorf("fallback caveats not appended: got %d, base %d", len(caps.Caveats), len(base.Caveats))
	}
}

func TestUpstreamSourceHeader(t *testing.T) {
	t.Parallel()

	out := &orchestrator.Outcome{
		Ready: true,
		Body:  json2(7.5),
		Provenance: model.Provenance{
			Source: model.SourceUpstream, Commit: "up", Date: "2026-08-05T12:00:00Z",
			ScorecardVersion: "v5.5.0", Complete: false,
		},
	}
	srv := New(&fakeProducer{out: out}, DefaultCapabilities(time.Hour).WithFallback())
	rec := do(t, srv, http.MethodGet, "/projects/github.com/ossf/scorecard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if src := rec.Header().Get(headerSource); src != "upstream" {
		t.Errorf("%s = %q, want upstream", headerSource, src)
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{}, DefaultCapabilities(time.Hour))
	if rec := do(t, srv, http.MethodGet, "/health"); rec.Code != http.StatusOK {
		t.Fatalf("/health status = %d, want 200", rec.Code)
	}
}

func TestReadyzDefault(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{}, DefaultCapabilities(time.Hour))
	if rec := do(t, srv, http.MethodGet, "/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200 by default", rec.Code)
	}
}

func TestReadyzNotReady(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{}, DefaultCapabilities(time.Hour),
		WithReadiness(func(context.Context) error { return errors.New("store unavailable") }))
	if rec := do(t, srv, http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz status = %d, want 503 when not ready", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()

	srv := New(&fakeProducer{out: liveOutcome(8.0)}, DefaultCapabilities(time.Hour))
	rec := do(t, srv, http.MethodPost, "/projects/github.com/ossf/scorecard")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", rec.Code)
	}
}
