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

// Package config loads all server configuration from the environment (design
// D10, 12-factor). Nothing cloud-specific is compiled in: the object store, TTL,
// timeouts, concurrency, listen address, and SCM credentials all come from env
// vars, each with a documented default. Required config that is missing or
// invalid fails fast at startup.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Environment variable names.
const (
	EnvBucketURL       = "SCORECARD_RESULTS_BUCKET_URL"
	EnvListenAddr      = "SCORECARD_LISTEN_ADDR"
	EnvPort            = "PORT"
	EnvLatestTTL       = "SCORECARD_LATEST_TTL"
	EnvSyncTimeout     = "SCORECARD_SYNC_TIMEOUT"
	EnvScanTimeout     = "SCORECARD_SCAN_TIMEOUT"
	EnvRetryAfter      = "SCORECARD_RETRY_AFTER"
	EnvConcurrency     = "SCORECARD_SCAN_CONCURRENCY"
	EnvLogLevel        = "SCORECARD_LOG_LEVEL"
	EnvEnabledCheck    = "SCORECARD_ENABLED_CHECKS"
	EnvGitHubTokens    = "SCORECARD_GITHUB_TOKENS"
	EnvGitHubAuth      = "GITHUB_AUTH_TOKEN"
	EnvHostRate        = "SCORECARD_HOST_RATE_PER_SECOND"
	EnvHostBurst       = "SCORECARD_HOST_RATE_BURST"
	EnvFlagsProvider   = "SCORECARD_FLAGS_PROVIDER"
	EnvFallbackURL     = "SCORECARD_FALLBACK_URL"
	EnvFallbackTimeout = "SCORECARD_FALLBACK_TIMEOUT"
	EnvFallbackMaxAge  = "SCORECARD_FALLBACK_MAX_AGE"
)

var (
	errMissingRequired = errors.New("config: required value missing")
	errInvalidValue    = errors.New("config: invalid value")
)

// Config is the fully-resolved server configuration.
type Config struct {
	// BucketURL is the gocloud.dev/blob URL for the result store (required).
	BucketURL string
	// ListenAddr is the HTTP listen address (default ":8080").
	ListenAddr string
	// LogLevel is the log level: debug, info, warn, or error (default "info").
	LogLevel string
	// FlagsProvider selects the feature-flag provider (default "static"); the
	// flags package validates the value at startup.
	FlagsProvider string
	// FallbackURL is the upstream Scorecard API base URL for the result fallback;
	// empty disables the fallback. The tier is further gated by the
	// fallback.enabled feature flag.
	FallbackURL string
	// EnabledChecks optionally restricts the checks to run; empty means all.
	EnabledChecks []string
	// GitHubTokens is the SCM token pool; falls back to GITHUB_AUTH_TOKEN.
	GitHubTokens []string
	// LatestTTL is how long a latest result stays fresh (default 24h).
	LatestTTL time.Duration
	// SyncTimeout bounds how long a request waits before a 202 (default 20s).
	SyncTimeout time.Duration
	// ScanTimeout bounds a background scan (default 5m).
	ScanTimeout time.Duration
	// RetryAfter is the hint returned with a 202 (default 10s).
	RetryAfter time.Duration
	// FallbackTimeout bounds a single upstream fallback fetch (default 5s).
	FallbackTimeout time.Duration
	// FallbackMaxAge is the maximum age of an upstream result that may be used or
	// backfilled (default 7d, aligned with the weekly public cron).
	FallbackMaxAge time.Duration
	// HostRatePerSecond is the per-host scan rate; 0 means unlimited (default 0).
	HostRatePerSecond float64
	// Concurrency bounds simultaneous live scans (default 4).
	Concurrency int
	// HostRateBurst is the per-host rate burst (default 1).
	HostRateBurst int
}

// Load resolves configuration using getenv (pass os.Getenv in production). It
// applies defaults, then validates, returning an actionable error on the first
// problem.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		ListenAddr:      ":8080",
		LogLevel:        "info",
		LatestTTL:       24 * time.Hour,
		SyncTimeout:     20 * time.Second,
		ScanTimeout:     5 * time.Minute,
		RetryAfter:      10 * time.Second,
		Concurrency:     4,
		HostRateBurst:   1,
		FlagsProvider:   "static",
		FallbackTimeout: 5 * time.Second,
		FallbackMaxAge:  7 * 24 * time.Hour,
	}

	c.BucketURL = getenv(EnvBucketURL)
	if c.BucketURL == "" {
		return Config{}, fmt.Errorf("%w: %s (e.g. file:///var/scorecard, s3://bucket, gs://bucket, mem://)",
			errMissingRequired, EnvBucketURL)
	}

	c.ListenAddr = listenAddr(getenv, c.ListenAddr)
	if v := getenv(EnvLogLevel); v != "" {
		c.LogLevel = v
	}
	if v := getenv(EnvFlagsProvider); v != "" {
		c.FlagsProvider = v
	}
	c.FallbackURL = getenv(EnvFallbackURL)
	c.EnabledChecks = splitList(getenv(EnvEnabledCheck))
	c.GitHubTokens = splitList(getenv(EnvGitHubTokens))
	if len(c.GitHubTokens) == 0 {
		c.GitHubTokens = splitList(getenv(EnvGitHubAuth))
	}

	var err error
	if c.LatestTTL, err = duration(getenv, EnvLatestTTL, c.LatestTTL); err != nil {
		return Config{}, err
	}
	if c.SyncTimeout, err = duration(getenv, EnvSyncTimeout, c.SyncTimeout); err != nil {
		return Config{}, err
	}
	if c.ScanTimeout, err = duration(getenv, EnvScanTimeout, c.ScanTimeout); err != nil {
		return Config{}, err
	}
	if c.RetryAfter, err = duration(getenv, EnvRetryAfter, c.RetryAfter); err != nil {
		return Config{}, err
	}
	if c.FallbackTimeout, err = duration(getenv, EnvFallbackTimeout, c.FallbackTimeout); err != nil {
		return Config{}, err
	}
	if c.FallbackMaxAge, err = duration(getenv, EnvFallbackMaxAge, c.FallbackMaxAge); err != nil {
		return Config{}, err
	}
	if c.Concurrency, err = positiveInt(getenv, EnvConcurrency, c.Concurrency); err != nil {
		return Config{}, err
	}
	if c.HostRateBurst, err = positiveInt(getenv, EnvHostBurst, c.HostRateBurst); err != nil {
		return Config{}, err
	}
	if c.HostRatePerSecond, err = nonNegativeFloat(getenv, EnvHostRate, c.HostRatePerSecond); err != nil {
		return Config{}, err
	}

	return c, validate(&c)
}

// listenAddr resolves the listen address, honoring PORT (e.g. Cloud Run) when an
// explicit address is not set.
func listenAddr(getenv func(string) string, def string) string {
	if v := getenv(EnvListenAddr); v != "" {
		return v
	}
	if p := getenv(EnvPort); p != "" {
		return ":" + p
	}
	return def
}

// validate checks the fully-resolved config for internal consistency.
func validate(c *Config) error {
	if c.FallbackURL != "" {
		u, err := url.Parse(c.FallbackURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: %s must be an http(s) URL", errInvalidValue, EnvFallbackURL)
		}
	}
	switch {
	case c.SyncTimeout <= 0:
		return fmt.Errorf("%w: %s must be > 0", errInvalidValue, EnvSyncTimeout)
	case c.ScanTimeout <= 0:
		return fmt.Errorf("%w: %s must be > 0", errInvalidValue, EnvScanTimeout)
	case c.LatestTTL < 0:
		return fmt.Errorf("%w: %s must be >= 0", errInvalidValue, EnvLatestTTL)
	case c.FallbackTimeout <= 0:
		return fmt.Errorf("%w: %s must be > 0", errInvalidValue, EnvFallbackTimeout)
	case c.FallbackMaxAge <= 0:
		return fmt.Errorf("%w: %s must be > 0", errInvalidValue, EnvFallbackMaxAge)
	default:
		return nil
	}
}

// splitList parses a comma-separated list, trimming spaces and dropping blanks.
func splitList(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// duration parses a Go duration env var, returning def when unset.
func duration(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q is not a duration: %w", errInvalidValue, key, v, err)
	}
	return d, nil
}

// positiveInt parses an integer env var that must be > 0, returning def when unset.
func positiveInt(getenv func(string) string, key string, def int) (int, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q is not an integer: %w", errInvalidValue, key, v, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%w: %s must be > 0, got %d", errInvalidValue, key, n)
	}
	return n, nil
}

// nonNegativeFloat parses a float env var that must be >= 0, returning def when unset.
func nonNegativeFloat(getenv func(string) string, key string, def float64) (float64, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q is not a number: %w", errInvalidValue, key, v, err)
	}
	if f < 0 {
		return 0, fmt.Errorf("%w: %s must be >= 0, got %v", errInvalidValue, key, f)
	}
	return f, nil
}
