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

package config

import (
	"errors"
	"testing"
	"time"
)

// env returns a getenv function backed by m, so tests need not mutate the
// process environment and can run in parallel.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	c, err := Load(env(map[string]string{EnvBucketURL: "mem://"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.BucketURL != "mem://" {
		t.Errorf("BucketURL = %q", c.BucketURL)
	}
	if c.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", c.ListenAddr)
	}
	if c.LatestTTL != 24*time.Hour || c.SyncTimeout != 20*time.Second || c.ScanTimeout != 5*time.Minute {
		t.Errorf("unexpected default timeouts: %+v", c)
	}
	if c.Concurrency != 4 || c.HostRateBurst != 1 || c.HostRatePerSecond != 0 {
		t.Errorf("unexpected default limits: %+v", c)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", c.LogLevel)
	}
	if c.FlagsProvider != "static" {
		t.Errorf("FlagsProvider = %q, want static", c.FlagsProvider)
	}
	if c.FallbackURL != "" {
		t.Errorf("FallbackURL = %q, want empty (disabled)", c.FallbackURL)
	}
	if c.FallbackTimeout != 5*time.Second || c.FallbackMaxAge != 7*24*time.Hour {
		t.Errorf("unexpected fallback defaults: timeout=%v maxAge=%v", c.FallbackTimeout, c.FallbackMaxAge)
	}
}

func TestFallbackConfig(t *testing.T) {
	t.Parallel()

	c, err := Load(env(map[string]string{
		EnvBucketURL:       "mem://",
		EnvFallbackURL:     "https://api.scorecard.dev",
		EnvFallbackTimeout: "3s",
		EnvFallbackMaxAge:  "48h",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.FallbackURL != "https://api.scorecard.dev" {
		t.Errorf("FallbackURL = %q", c.FallbackURL)
	}
	if c.FallbackTimeout != 3*time.Second || c.FallbackMaxAge != 48*time.Hour {
		t.Errorf("unexpected fallback values: timeout=%v maxAge=%v", c.FallbackTimeout, c.FallbackMaxAge)
	}
}

func TestLoadMissingBucketFailsFast(t *testing.T) {
	t.Parallel()

	if _, err := Load(env(nil)); !errors.Is(err, errMissingRequired) {
		t.Fatalf("Load without bucket = %v, want errMissingRequired", err)
	}
}

func TestLoadCustom(t *testing.T) {
	t.Parallel()

	c, err := Load(env(map[string]string{
		EnvBucketURL:    "s3://bucket?region=us-east-1",
		EnvListenAddr:   ":9090",
		EnvLatestTTL:    "1h",
		EnvSyncTimeout:  "5s",
		EnvScanTimeout:  "2m",
		EnvRetryAfter:   "15s",
		EnvConcurrency:  "8",
		EnvLogLevel:     "debug",
		EnvEnabledCheck: "Maintained, Code-Review ,",
		EnvGitHubTokens: "t1,t2",
		EnvHostRate:     "2.5",
		EnvHostBurst:    "3",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":9090" || c.LatestTTL != time.Hour || c.SyncTimeout != 5*time.Second {
		t.Errorf("unexpected values: %+v", c)
	}
	if c.Concurrency != 8 || c.HostRatePerSecond != 2.5 || c.HostRateBurst != 3 {
		t.Errorf("unexpected limits: %+v", c)
	}
	if len(c.EnabledChecks) != 2 || c.EnabledChecks[0] != "Maintained" || c.EnabledChecks[1] != "Code-Review" {
		t.Errorf("EnabledChecks = %v, want [Maintained Code-Review]", c.EnabledChecks)
	}
	if len(c.GitHubTokens) != 2 {
		t.Errorf("GitHubTokens = %v, want 2", c.GitHubTokens)
	}
}

func TestPortFallback(t *testing.T) {
	t.Parallel()

	c, err := Load(env(map[string]string{EnvBucketURL: "mem://", EnvPort: "3000"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != ":3000" {
		t.Errorf("ListenAddr = %q, want :3000 from PORT", c.ListenAddr)
	}
}

func TestGitHubAuthFallback(t *testing.T) {
	t.Parallel()

	c, err := Load(env(map[string]string{EnvBucketURL: "mem://", EnvGitHubAuth: "abc,def"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.GitHubTokens) != 2 {
		t.Errorf("GitHubTokens = %v, want 2 from GITHUB_AUTH_TOKEN fallback", c.GitHubTokens)
	}
}

func TestLoadInvalid(t *testing.T) {
	t.Parallel()

	cases := map[string]map[string]string{
		"bad duration":     {EnvBucketURL: "mem://", EnvLatestTTL: "soon"},
		"bad concurrency":  {EnvBucketURL: "mem://", EnvConcurrency: "-1"},
		"non-int burst":    {EnvBucketURL: "mem://", EnvHostBurst: "lots"},
		"bad rate":         {EnvBucketURL: "mem://", EnvHostRate: "-2"},
		"bad fallback url": {EnvBucketURL: "mem://", EnvFallbackURL: "notaurl"},
		"zero fb timeout":  {EnvBucketURL: "mem://", EnvFallbackTimeout: "0s"},
		"zero fb max age":  {EnvBucketURL: "mem://", EnvFallbackMaxAge: "0s"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(env(m)); !errors.Is(err, errInvalidValue) {
				t.Fatalf("Load(%v) = %v, want errInvalidValue", m, err)
			}
		})
	}
}
