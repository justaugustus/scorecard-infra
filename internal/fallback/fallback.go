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

// Package fallback fetches an existing latest Scorecard result from an upstream
// Scorecard API (e.g. api.scorecard.dev) over the same /projects GET contract
// this server exposes (design F8). It is best-effort and read-only: a missing or
// too-old result, or any transport error, is reported so the orchestrator can
// fall through to a live scan.
package fallback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/uwu-tools/scorecard-infra/internal/model"
	"github.com/uwu-tools/scorecard-infra/internal/scan"
)

// ErrFallbackMiss means the upstream has no usable result for the repository —
// either no result at all, or one older than the configured maximum age. The
// orchestrator treats it as "fall through", distinct from a fetch error.
var ErrFallbackMiss = errors.New("fallback: no usable upstream result")

// errUpstreamStatus is wrapped when the upstream returns an unexpected HTTP status.
var errUpstreamStatus = errors.New("fallback: unexpected upstream status")

// maxResponseBytes bounds an upstream response so a misbehaving endpoint cannot
// exhaust memory. Scorecard JSON2 results are tens of KB; 8 MiB is generous.
const maxResponseBytes = 8 << 20

// Client fetches results from an upstream Scorecard API over the /projects
// contract.
type Client struct {
	httpClient *http.Client
	now        func() time.Time
	baseURL    string
	maxAge     time.Duration
}

// Option customizes a Client (used mainly by tests).
type Option func(*Client)

// WithClock overrides the time source used for the max-age check.
func WithClock(now func() time.Time) Option {
	return func(c *Client) { c.now = now }
}

// NewClient builds a Client for baseURL (e.g. "https://api.scorecard.dev").
// timeout bounds a single fetch; maxAge bounds how old an upstream result may be.
func NewClient(baseURL string, timeout, maxAge time.Duration, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: timeout},
		now:        time.Now,
		baseURL:    strings.TrimRight(baseURL, "/"),
		maxAge:     maxAge,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Fetch retrieves the upstream latest result for ref. It returns ErrFallbackMiss
// when the upstream has no result (404) or the result is older than maxAge, and a
// wrapped error for any transport or protocol failure.
func (c *Client) Fetch(ctx context.Context, ref model.RepoRef) (*scan.Result, error) {
	endpoint := c.baseURL + "/projects/" + ref.Host + "/" + ref.Org + "/" + ref.Repo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("fallback: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fallback: requesting %s: %w", ref.Name(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through and parse
	case http.StatusNotFound:
		return nil, ErrFallbackMiss
	default:
		return nil, fmt.Errorf("%w %d for %s", errUpstreamStatus, resp.StatusCode, ref.Name())
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("fallback: reading response for %s: %w", ref.Name(), err)
	}
	parsed, err := model.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("fallback: parsing upstream result for %s: %w", ref.Name(), err)
	}
	if c.stale(parsed.Date) {
		return nil, ErrFallbackMiss
	}
	return &scan.Result{
		JSON2:          body,
		ResolvedCommit: parsed.Repo.Commit,
		Complete:       scan.Complete(parsed.Checks),
	}, nil
}

// stale reports whether an upstream result's generation date is older than maxAge.
// An unparseable date is treated as stale: freshness cannot be confirmed, so the
// result is not used.
func (c *Client) stale(date string) bool {
	t, err := time.Parse(time.RFC3339, date)
	if err != nil {
		return true
	}
	return c.now().Sub(t) > c.maxAge
}
