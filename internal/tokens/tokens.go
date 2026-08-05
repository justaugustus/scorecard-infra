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

// Package tokens manages SCM credentials and rate limiting for live scans
// (design D8). It provides a token pool, a per-host rate limiter, and
// exponential backoff/retry.
//
// SCM API rate limits — not CPU — are the scaling bottleneck for live scanning.
// Scorecard's own GitHub roundtripper already rotates a comma-separated set of
// tokens (GITHUB_AUTH_TOKEN) thread-safely across concurrent requests; PATPool
// models that set (and can render it via Joined) while HostLimiter and Retry add
// per-host pacing and resilient retries at this server's orchestration layer.
package tokens

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// PATPool hands out SCM personal access tokens in round-robin order. It is safe
// for concurrent use. An empty pool yields the empty string (unauthenticated
// access), which callers may treat as a configuration warning.
type PATPool struct {
	tokens []string
	mu     sync.Mutex
	next   int
}

// NewPATPool builds a pool from the given tokens, dropping blank entries.
func NewPATPool(tokens []string) *PATPool {
	cleaned := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return &PATPool{tokens: cleaned}
}

// Len reports how many tokens the pool holds.
func (p *PATPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.tokens)
}

// Next returns the next token in round-robin order, or "" when the pool is empty.
func (p *PATPool) Next() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tokens) == 0 {
		return ""
	}
	t := p.tokens[p.next]
	p.next = (p.next + 1) % len(p.tokens)
	return t
}

// Joined returns the tokens as a comma-separated string, the form Scorecard's
// GitHub roundtripper consumes via GITHUB_AUTH_TOKEN for built-in rotation.
func (p *PATPool) Joined() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.tokens, ",")
}

// HostLimiter applies an independent token-bucket rate limit per SCM host, so a
// burst of scans against one host cannot exhaust that host's API quota.
type HostLimiter struct {
	limiters map[string]*rate.Limiter
	limit    rate.Limit
	mu       sync.Mutex
	burst    int
}

// NewHostLimiter returns a limiter allowing perSecond events per host with the
// given burst. A non-positive perSecond means unlimited; burst is floored at 1.
func NewHostLimiter(perSecond float64, burst int) *HostLimiter {
	limit := rate.Limit(perSecond)
	if perSecond <= 0 {
		limit = rate.Inf
	}
	if burst < 1 {
		burst = 1
	}
	return &HostLimiter{
		limiters: make(map[string]*rate.Limiter),
		limit:    limit,
		burst:    burst,
	}
}

// Wait blocks until the limiter for host permits an event or ctx is done.
func (h *HostLimiter) Wait(ctx context.Context, host string) error {
	if err := h.limiterFor(host).Wait(ctx); err != nil {
		return fmt.Errorf("tokens: rate-limit wait for %q: %w", host, err)
	}
	return nil
}

func (h *HostLimiter) limiterFor(host string) *rate.Limiter {
	h.mu.Lock()
	defer h.mu.Unlock()
	l, ok := h.limiters[host]
	if !ok {
		l = rate.NewLimiter(h.limit, h.burst)
		h.limiters[host] = l
	}
	return l
}

// BackoffConfig configures the exponential backoff used by Retry.
type BackoffConfig struct {
	// MaxAttempts is the total number of calls to make (>= 1).
	MaxAttempts int
	// Base is the delay before the second attempt; it doubles each attempt.
	Base time.Duration
	// Max caps the per-attempt delay (0 means uncapped).
	Max time.Duration
}

// DefaultBackoff returns a reasonable backoff policy for SCM API calls.
func DefaultBackoff() BackoffConfig {
	return BackoffConfig{
		MaxAttempts: 4,
		Base:        500 * time.Millisecond,
		Max:         30 * time.Second,
	}
}

// permanent wraps an error to signal that Retry must not retry it.
type permanent struct{ err error }

func (p *permanent) Error() string { return p.err.Error() }

func (p *permanent) Unwrap() error { return p.err }

// Permanent marks err as non-retryable: Retry returns it (unwrapped) at once.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanent{err: err}
}

// errNoAttempts is returned when Retry is called with MaxAttempts < 1.
var errNoAttempts = errors.New("tokens: MaxAttempts must be >= 1")

// Retry calls fn with exponential backoff until it succeeds, returns a Permanent
// error, exhausts MaxAttempts, or ctx is done, whichever comes first. It returns
// nil on success and otherwise the last (unwrapped) error.
func Retry(ctx context.Context, cfg BackoffConfig, fn func() error) error {
	if cfg.MaxAttempts < 1 {
		return errNoAttempts
	}
	var lastErr error
	delay := cfg.Base
	for attempt := range cfg.MaxAttempts {
		if attempt > 0 {
			if err := sleep(ctx, delay); err != nil {
				return err
			}
			delay *= 2
			if cfg.Max > 0 && delay > cfg.Max {
				delay = cfg.Max
			}
		}
		err := fn()
		if err == nil {
			return nil
		}
		var perm *permanent
		if errors.As(err, &perm) {
			return perm.err
		}
		lastErr = err
	}
	return lastErr
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("tokens: retry aborted: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
