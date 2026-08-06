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

	"github.com/uwu-tools/scorecard-api/internal/fallback"
	"github.com/uwu-tools/scorecard-api/internal/model"
	"github.com/uwu-tools/scorecard-api/internal/scan"
	"github.com/uwu-tools/scorecard-api/internal/store"
)

// Defaults applied by New when a Config field is unset.
const (
	defaultSyncTimeout    = 20 * time.Second
	defaultScanTimeout    = 5 * time.Minute
	defaultRetryAfter     = 10 * time.Second
	defaultFallbackMaxAge = 7 * 24 * time.Hour
)

// errUnexpectedType guards the singleflight value type assertion; it should be
// unreachable.
var errUnexpectedType = errors.New("orchestrator: unexpected singleflight value type")

// Feature-flag keys and values that gate the upstream fallback (design F2/F3).
const (
	// FlagEnabled toggles the upstream fallback at runtime (default true when a
	// fallback is configured).
	FlagEnabled = "fallback.enabled"
	// FlagMode selects the fallback ordering.
	FlagMode = "fallback.mode"
	// ModeFetchFirst consults the upstream before scanning (the default).
	ModeFetchFirst = "fetch-first"
	// ModeSafetyNet scans first and consults the upstream only on a scan failure.
	ModeSafetyNet = "safety-net"
)

// Fallback fetches an existing latest result from an upstream Scorecard API. The
// orchestrator consults it only for latest requests, per the configured mode.
type Fallback interface {
	Fetch(ctx context.Context, ref model.RepoRef) (*scan.Result, error)
}

// FlagSource evaluates the feature flags that gate the fallback. *flags.Flags
// satisfies it; a nil FlagSource uses the coded defaults.
type FlagSource interface {
	Bool(ctx context.Context, key string, def bool) bool
	String(ctx context.Context, key, def string) string
}

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
	// FallbackMaxAge is how long an upstream-sourced result stays fresh and the
	// bound for using or backfilling one. Zero uses a 7-day default.
	FallbackMaxAge time.Duration
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
	store          *store.Store
	scanner        scan.Scanner
	fallback       Fallback
	flags          FlagSource
	pool           *scan.Bounded
	logger         *slog.Logger
	now            func() time.Time
	group          singleflight.Group
	ttl            time.Duration
	syncTimeout    time.Duration
	scanTimeout    time.Duration
	retryAfter     time.Duration
	fallbackMaxAge time.Duration
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

// WithFallback enables the upstream result fallback tier. A nil fallback (the
// default) disables it.
func WithFallback(f Fallback) Option {
	return func(o *Orchestrator) { o.fallback = f }
}

// WithFlags sets the feature-flag source that gates the fallback. A nil source
// (the default) uses the coded defaults (enabled, fetch-first).
func WithFlags(f FlagSource) Option {
	return func(o *Orchestrator) { o.flags = f }
}

// New builds an Orchestrator over the given store and scanner.
func New(st *store.Store, sc scan.Scanner, cfg Config, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		store:          st,
		scanner:        sc,
		pool:           scan.NewBounded(cfg.Concurrency),
		logger:         slog.Default(),
		now:            time.Now,
		ttl:            cfg.TTL,
		syncTimeout:    cfg.SyncTimeout,
		scanTimeout:    cfg.ScanTimeout,
		retryAfter:     cfg.RetryAfter,
		fallbackMaxAge: cfg.FallbackMaxAge,
	}
	if o.fallbackMaxAge <= 0 {
		o.fallbackMaxAge = defaultFallbackMaxAge
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
	body, origin, err := o.store.GetWithOrigin(ctx, ref, commit)
	switch {
	case err == nil:
		// Commit-pinned results are immutable; latest results honor their
		// source-aware freshness window.
		if commit != "" || o.fresh(body, origin) {
			return o.cachedOutcome(body, origin)
		}
	case errors.Is(err, store.ErrNotFound):
		// Cache miss: fall through to produce.
	default:
		return nil, fmt.Errorf("orchestrator: store lookup: %w", err)
	}

	return o.produce(ctx, ref, commit)
}

// fresh reports whether a stored latest result is within its freshness window.
// The window is source-aware: a locally-scanned result ages by the latest TTL,
// an upstream-sourced result by the fallback max-age (design F6).
func (o *Orchestrator) fresh(body []byte, origin store.Origin) bool {
	parsed, err := model.Parse(body)
	if err != nil {
		return false
	}
	date, err := time.Parse(time.RFC3339, parsed.Date)
	if err != nil {
		return false
	}
	window := o.ttl
	if origin == store.OriginUpstream {
		window = o.fallbackMaxAge
	}
	return o.now().Sub(date) <= window
}

// cachedOutcome builds a ready outcome from a stored body, declaring its true
// source: cached for a locally-scanned entry, upstream for a backfilled one.
func (o *Orchestrator) cachedOutcome(body []byte, origin store.Origin) (*Outcome, error) {
	parsed, err := model.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parsing cached result: %w", err)
	}
	src := model.SourceCached
	if origin == store.OriginUpstream {
		src = model.SourceUpstream
	}
	return &Outcome{
		Provenance: model.ProvenanceFrom(parsed, src, scan.Complete(parsed.Checks)),
		Body:       body,
		Ready:      true,
	}, nil
}

// produce serves a result on a cache miss/stale, weaving in the upstream fallback
// per the active mode (design F3): fetch-first consults the upstream before
// scanning; safety-net scans first and consults the upstream only on a scan
// failure. When the fallback is inactive it is a plain scan.
func (o *Orchestrator) produce(ctx context.Context, ref model.RepoRef, commit string) (*Outcome, error) {
	mode, active := o.fallbackDecision(ctx, commit)
	if active && mode == ModeFetchFirst {
		if out := o.tryFallback(ctx, ref); out != nil {
			return out, nil
		}
	}
	outcome, err := o.scanOutcome(ctx, ref, commit)
	if err != nil && active && mode == ModeSafetyNet {
		if out := o.tryFallback(ctx, ref); out != nil {
			return out, nil
		}
	}
	return outcome, err
}

// fallbackDecision reports the active fallback mode for a request, or active=false
// when the fallback is not configured, disabled by flag, or the request is
// commit-pinned (the upstream answers only latest, design F7).
func (o *Orchestrator) fallbackDecision(ctx context.Context, commit string) (string, bool) {
	if o.fallback == nil || commit != "" {
		return "", false
	}
	if o.flags != nil && !o.flags.Bool(ctx, FlagEnabled, true) {
		return "", false
	}
	mode := ModeFetchFirst
	if o.flags != nil {
		mode = o.flags.String(ctx, FlagMode, ModeFetchFirst)
	}
	return mode, true
}

// tryFallback fetches an upstream result, backfills it tagged as upstream, and
// returns a ready outcome. It returns nil on a miss or any error (both non-fatal,
// so the caller falls through), logging a genuine fetch failure.
func (o *Orchestrator) tryFallback(ctx context.Context, ref model.RepoRef) *Outcome {
	res, err := o.fallback.Fetch(ctx, ref)
	if err != nil {
		if !errors.Is(err, fallback.ErrFallbackMiss) {
			o.logger.Debug("upstream fallback fetch failed", "ref", ref.Name(), "error", err)
		}
		return nil
	}
	o.backfill(ctx, ref, res)
	parsed, err := model.Parse(res.JSON2)
	if err != nil {
		o.logger.Warn("parsing upstream result", "ref", ref.Name(), "error", err)
		return nil
	}
	return &Outcome{
		Provenance: model.ProvenanceFrom(parsed, model.SourceUpstream, res.Complete),
		Body:       res.JSON2,
		Ready:      true,
	}
}

// backfill persists an upstream result tagged as upstream so later requests serve
// it (aged by the fallback max-age, design F6). Failure is non-fatal.
func (o *Orchestrator) backfill(ctx context.Context, ref model.RepoRef, res *scan.Result) {
	var err error
	if res.ResolvedCommit == "" {
		err = o.store.PutWithOrigin(ctx, ref, "", res.JSON2, store.OriginUpstream)
	} else {
		err = o.store.PutLatestAndCommitWithOrigin(ctx, ref, res.ResolvedCommit, res.JSON2, store.OriginUpstream)
	}
	if err != nil {
		o.logger.Warn("upstream backfill failed", "ref", ref.Name(), "error", err)
	}
}

// scanOutcome coalesces concurrent identical requests into one scan (D6) and
// returns synchronously if the scan finishes within SyncTimeout, else a not-ready
// outcome while the scan continues in the background (D5).
func (o *Orchestrator) scanOutcome(ctx context.Context, ref model.RepoRef, commit string) (*Outcome, error) {
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
