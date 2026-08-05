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

// Package orchestrator is the read-through cache and the central serve-vs-scan
// seam (design D2). Every request flows through GetOrProduce, which looks up the
// store, checks freshness, and either serves the cached result or triggers a
// live scan, persists it, and returns it.
//
// It owns the freshness policy (commit-pinned = immutable; latest = TTL, D5),
// single-flight de-duplication so concurrent identical requests trigger exactly
// one scan (D6), the synchronous-with-timeout vs. asynchronous decision (D5),
// and provenance stamping on every result (D12).
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/uwu-tools/scorecard-api/internal/model"
	"github.com/uwu-tools/scorecard-api/internal/scan"
	"github.com/uwu-tools/scorecard-api/internal/store"
)

// Defaults applied by New when a Config field is unset.
const (
	defaultSyncTimeout = 20 * time.Second
	defaultScanTimeout = 5 * time.Minute
	defaultRetryAfter  = 10 * time.Second
)

// errUnexpectedType guards the singleflight value type assertion; it should be
// unreachable.
var errUnexpectedType = errors.New("orchestrator: unexpected singleflight value type")

// Config tunes the orchestrator's freshness and response policy.
type Config struct {
	// TTL is how long a latest result stays fresh. Zero means always refresh.
	TTL time.Duration
	// SyncTimeout bounds how long a request waits for an in-flight scan before
	// returning a not-ready (202) response.
	SyncTimeout time.Duration
	// ScanTimeout bounds a background scan, independent of any single request.
	ScanTimeout time.Duration
	// RetryAfter is the hint returned with a not-ready response.
	RetryAfter time.Duration
	// Concurrency bounds simultaneous live scans.
	Concurrency int
}

// Outcome is the result of GetOrProduce. When Ready is true the Body and
// Provenance are populated (HTTP 200); when false a scan is in flight and the
// client should retry after RetryAfter (HTTP 202).
type Outcome struct {
	Provenance model.Provenance
	Body       []byte
	RetryAfter time.Duration
	Ready      bool
}

// Orchestrator is the read-through cache over the store and scanner.
type Orchestrator struct {
	store       *store.Store
	scanner     scan.Scanner
	pool        *scan.Bounded
	logger      *slog.Logger
	now         func() time.Time
	group       singleflight.Group
	ttl         time.Duration
	syncTimeout time.Duration
	scanTimeout time.Duration
	retryAfter  time.Duration
}

// Option customizes an Orchestrator (used mainly by tests).
type Option func(*Orchestrator)

// WithClock overrides the time source used for freshness checks.
func WithClock(now func() time.Time) Option {
	return func(o *Orchestrator) { o.now = now }
}

// WithLogger overrides the logger used for non-fatal warnings.
func WithLogger(l *slog.Logger) Option {
	return func(o *Orchestrator) { o.logger = l }
}

// New builds an Orchestrator over the given store and scanner.
func New(st *store.Store, sc scan.Scanner, cfg Config, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		store:       st,
		scanner:     sc,
		pool:        scan.NewBounded(cfg.Concurrency),
		logger:      slog.Default(),
		now:         time.Now,
		ttl:         cfg.TTL,
		syncTimeout: cfg.SyncTimeout,
		scanTimeout: cfg.ScanTimeout,
		retryAfter:  cfg.RetryAfter,
	}
	if o.syncTimeout <= 0 {
		o.syncTimeout = defaultSyncTimeout
	}
	if o.scanTimeout <= 0 {
		o.scanTimeout = defaultScanTimeout
	}
	if o.retryAfter <= 0 {
		o.retryAfter = defaultRetryAfter
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// GetOrProduce serves a fresh cached result or produces one on demand. An empty
// commit requests the latest result; a non-empty commit is immutable. It returns
// scan.ErrSkipped when the repository cannot be scanned for a non-fatal reason.
func (o *Orchestrator) GetOrProduce(ctx context.Context, ref model.RepoRef, commit string) (*Outcome, error) {
	body, err := o.store.Get(ctx, ref, commit)
	switch {
	case err == nil:
		// Commit-pinned results are immutable; latest results honor the TTL.
		if commit != "" || o.fresh(body) {
			return o.cachedOutcome(body)
		}
	case errors.Is(err, store.ErrNotFound):
		// Cache miss: fall through to produce.
	default:
		return nil, fmt.Errorf("orchestrator: store lookup: %w", err)
	}

	return o.produce(ctx, ref, commit)
}

// fresh reports whether a stored latest result is within its TTL.
func (o *Orchestrator) fresh(body []byte) bool {
	parsed, err := model.Parse(body)
	if err != nil {
		return false
	}
	date, err := time.Parse(time.RFC3339, parsed.Date)
	if err != nil {
		return false
	}
	return o.now().Sub(date) <= o.ttl
}

// cachedOutcome builds a ready outcome from a stored body, declaring it cached.
func (o *Orchestrator) cachedOutcome(body []byte) (*Outcome, error) {
	parsed, err := model.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parsing cached result: %w", err)
	}
	return &Outcome{
		Provenance: model.ProvenanceFrom(parsed, model.SourceCached, scan.Complete(parsed.Checks)),
		Body:       body,
		Ready:      true,
	}, nil
}

// produce coalesces concurrent identical requests into one scan (D6) and returns
// synchronously if the scan finishes within SyncTimeout, else a not-ready
// outcome while the scan continues in the background (D5).
func (o *Orchestrator) produce(ctx context.Context, ref model.RepoRef, commit string) (*Outcome, error) {
	key := store.Key(ref, commit)
	// scanAndStore intentionally runs on a background-scoped context so the scan
	// survives a request that returns 202 and keeps populating the cache.
	ch := o.group.DoChan(key, func() (any, error) { //nolint:contextcheck // background scan must outlive the request
		return o.scanAndStore(ref, commit)
	})

	timer := time.NewTimer(o.syncTimeout)
	defer timer.Stop()

	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		outcome, ok := res.Val.(*Outcome)
		if !ok {
			return nil, errUnexpectedType
		}
		return outcome, nil
	case <-timer.C:
		return &Outcome{Ready: false, RetryAfter: o.retryAfter}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("orchestrator: request cancelled: %w", ctx.Err())
	}
}

// scanAndStore runs one scan on a background-scoped context (so it survives the
// originating request), persists the result, and returns a live outcome.
func (o *Orchestrator) scanAndStore(ref model.RepoRef, commit string) (*Outcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), o.scanTimeout)
	defer cancel()

	var result *scan.Result
	if err := o.pool.Do(ctx, func() error {
		r, scanErr := o.scanner.Scan(ctx, ref, commit)
		if scanErr != nil {
			return scanErr
		}
		result = r
		return nil
	}); err != nil {
		return nil, err
	}

	o.persist(ctx, ref, commit, result)

	parsed, err := model.Parse(result.JSON2)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parsing produced result: %w", err)
	}
	return &Outcome{
		Provenance: model.ProvenanceFrom(parsed, model.SourceLive, result.Complete),
		Body:       result.JSON2,
		Ready:      true,
	}, nil
}

// persist writes the result back to the cache. A latest scan populates both the
// latest pointer and the resolved-commit key; a commit-pinned scan writes only
// its key. A write-back failure is logged, not fatal — serving the fresh result
// is better than failing the request.
func (o *Orchestrator) persist(ctx context.Context, ref model.RepoRef, commit string, result *scan.Result) {
	var err error
	if commit == "" {
		err = o.store.PutLatestAndCommit(ctx, ref, result.ResolvedCommit, result.JSON2)
	} else {
		err = o.store.Put(ctx, ref, commit, result.JSON2)
	}
	if err != nil {
		o.logger.Warn("cache write-back failed",
			"ref", ref.Name(), "commit", commit, "error", err)
	}
}
