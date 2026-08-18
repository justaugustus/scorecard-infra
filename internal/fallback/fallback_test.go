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

package fallback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ossf/scorecard-infra/internal/model"
)

func testRef(t *testing.T) model.RepoRef {
	t.Helper()
	ref, err := model.ParseRepoRef("github.com", "ossf", "scorecard")
	if err != nil {
		t.Fatalf("ParseRepoRef: %v", err)
	}
	return ref
}

// json2 builds a minimal canonical JSON2 body with the given date, commit, and score.
func json2(date time.Time, commit string, score float64) []byte {
	return fmt.Appendf(nil,
		`{"date":%q,"repo":{"name":"github.com/ossf/scorecard","commit":%q},`+
			`"scorecard":{"version":"v5.5.0","commit":"engine-sha"},"score":%.1f,"checks":[]}`,
		date.Format(time.RFC3339), commit, score)
}

func TestFetchHit(t *testing.T) {
	t.Parallel()

	body := json2(time.Now(), "abc123", 8.9)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/github.com/ossf/scorecard" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if _, err := w.Write(body); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, time.Second, 24*time.Hour).Fetch(context.Background(), testRef(t))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.ResolvedCommit != "abc123" || !bytes.Equal(got.JSON2, body) || !got.Complete {
		t.Fatalf("Fetch = %+v; want commit abc123, matching body, complete", got)
	}
}

func TestFetchMissOn404(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, time.Second, 24*time.Hour).Fetch(context.Background(), testRef(t))
	if !errors.Is(err, ErrFallbackMiss) {
		t.Fatalf("Fetch(404) = %v, want ErrFallbackMiss", err)
	}
}

func TestFetchMissWhenStale(t *testing.T) {
	t.Parallel()

	date := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(json2(date, "abc123", 8.9)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	// Clock is 48h past the result's date, exceeding the 24h max-age.
	clock := func() time.Time { return date.Add(48 * time.Hour) }
	_, err := NewClient(srv.URL, time.Second, 24*time.Hour, WithClock(clock)).Fetch(context.Background(), testRef(t))
	if !errors.Is(err, ErrFallbackMiss) {
		t.Fatalf("Fetch(stale) = %v, want ErrFallbackMiss", err)
	}
}

func TestFetchErrorOnUpstreamStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, time.Second, 24*time.Hour).Fetch(context.Background(), testRef(t))
	if err == nil || errors.Is(err, ErrFallbackMiss) {
		t.Fatalf("Fetch(500) = %v, want a non-miss error", err)
	}
}

func TestFetchErrorOnTransport(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // no listener now; the next request fails at transport

	_, err := NewClient(url, time.Second, 24*time.Hour).Fetch(context.Background(), testRef(t))
	if err == nil || errors.Is(err, ErrFallbackMiss) {
		t.Fatalf("Fetch(transport error) = %v, want a non-miss error", err)
	}
}
