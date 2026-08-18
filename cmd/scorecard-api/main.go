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

// Command scorecard-api is a cloud-agnostic, self-hosted OpenSSF Scorecard
// results API server. It serves pre-computed results from any object store and
// generates them live on demand — a read-through cache over an in-process scan
// engine ("hybrid"). It speaks the ossf/scorecard-webapp GET contract so it is a
// drop-in --base-url target for uwu-tools/scorecard-mcp.
//
// All configuration comes from the environment; see internal/config.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ossf/scorecard-infra/internal/config"
	"github.com/ossf/scorecard-infra/internal/fallback"
	"github.com/ossf/scorecard-infra/internal/flags"
	"github.com/ossf/scorecard-infra/internal/httpapi"
	"github.com/ossf/scorecard-infra/internal/orchestrator"
	"github.com/ossf/scorecard-infra/internal/scan"
	"github.com/ossf/scorecard-infra/internal/store"
	"github.com/ossf/scorecard-infra/internal/tokens"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "scorecard-api:", err)
		os.Exit(1)
	}
}

// run loads configuration, wires the store, scanner, orchestrator, and HTTP
// server, and serves until interrupted.
func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	// Initialize the feature-flag provider at startup (design FF2/FF7): this
	// validates SCORECARD_FLAGS_PROVIDER and the fallback flags, establishing the
	// in-process, env-seeded provider and failing fast on a bad value.
	fl, err := flags.New(os.Getenv, flags.Config{Provider: cfg.FlagsProvider}, fallbackFlagDefs()...)
	if err != nil {
		return fmt.Errorf("initializing feature flags: %w", err)
	}
	logger.Info("feature flags initialized", "provider", cfg.FlagsProvider)

	// Feed the SCM token pool into Scorecard's GitHub roundtripper, which rotates
	// a comma-separated GITHUB_AUTH_TOKEN across concurrent requests (design D8).
	pool := tokens.NewPATPool(cfg.GitHubTokens)
	if pool.Len() == 0 {
		logger.Warn("no SCM tokens configured; live scans will be unauthenticated and heavily rate-limited")
	} else if err := os.Setenv(config.EnvGitHubAuth, pool.Joined()); err != nil {
		return fmt.Errorf("setting %s: %w", config.EnvGitHubAuth, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.BucketURL)
	if err != nil {
		return fmt.Errorf("opening result store: %w", err)
	}
	defer closeQuietly(logger, "store", st.Close)

	limiter := tokens.NewHostLimiter(cfg.HostRatePerSecond, cfg.HostRateBurst)
	scanner, err := scan.NewEngineScanner(scan.EngineConfig{
		Limiter:  limiter,
		LogLevel: cfg.LogLevel,
		Checks:   cfg.EnabledChecks,
	})
	if err != nil {
		return fmt.Errorf("creating scanner: %w", err)
	}
	defer closeQuietly(logger, "scanner", scanner.Close)

	orchOpts := []orchestrator.Option{orchestrator.WithLogger(logger), orchestrator.WithFlags(fl)}
	caps := httpapi.DefaultCapabilities(cfg.LatestTTL)
	if cfg.FallbackURL != "" {
		fb := fallback.NewClient(cfg.FallbackURL, cfg.FallbackTimeout, cfg.FallbackMaxAge)
		orchOpts = append(orchOpts, orchestrator.WithFallback(fb))
		caps = caps.WithFallback()
		logger.Info("upstream fallback enabled", "url", cfg.FallbackURL, "max_age", cfg.FallbackMaxAge)
	}

	orch := orchestrator.New(st, scanner, orchestrator.Config{
		TTL:            cfg.LatestTTL,
		SyncTimeout:    cfg.SyncTimeout,
		ScanTimeout:    cfg.ScanTimeout,
		RetryAfter:     cfg.RetryAfter,
		Concurrency:    cfg.Concurrency,
		FallbackMaxAge: cfg.FallbackMaxAge,
	}, orchOpts...)

	srv := httpapi.New(orch, caps, httpapi.WithLogger(logger))

	logger.Info("starting scorecard-api",
		"version", version, "addr", cfg.ListenAddr, "bucket", cfg.BucketURL, "tokens", pool.Len())

	return srv.ListenAndServe(ctx, httpapi.ServeConfig{
		Addr:            cfg.ListenAddr,
		ShutdownTimeout: 10 * time.Second,
	})
}

// fallbackFlagDefs declares the feature flags that gate the upstream fallback, so
// the flags package validates their env overrides at startup. They are always
// registered; the orchestrator ignores them when no fallback is configured.
func fallbackFlagDefs() []flags.Definition {
	return []flags.Definition{
		{Key: orchestrator.FlagEnabled, Kind: flags.KindBool, Default: true},
		{
			Key: orchestrator.FlagMode, Kind: flags.KindString,
			Default: orchestrator.ModeFetchFirst,
			Allowed: []string{orchestrator.ModeFetchFirst, orchestrator.ModeSafetyNet},
		},
	}
}

// newLogger builds a JSON slog logger at the given level, defaulting to info.
func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

// closeQuietly runs a Close function on shutdown, logging any error.
func closeQuietly(logger *slog.Logger, name string, closeFn func() error) {
	if err := closeFn(); err != nil {
		logger.Error("closing "+name, "error", err)
	}
}
