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

package flags

import (
	"context"
	"testing"
)

// fakeEnv returns a getenv backed by a map.
func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// newForTest builds a Flags bound to a per-test domain for isolation.
func newForTest(t *testing.T, env map[string]string, defs ...Definition) *Flags {
	t.Helper()
	f, err := New(fakeEnv(env), Config{Domain: t.Name()}, defs...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func TestBoolDefaultWhenUnset(t *testing.T) {
	t.Parallel()
	f := newForTest(t, nil, Definition{Key: "fallback.enabled", Kind: KindBool, Default: false})
	if got := f.Bool(context.Background(), "fallback.enabled", true); got {
		t.Fatalf("Bool = %v, want false (the declared default)", got)
	}
}

func TestBoolEnvOverride(t *testing.T) {
	t.Parallel()
	env := map[string]string{"SCORECARD_FLAG_FALLBACK_ENABLED": "true"}
	f := newForTest(t, env, Definition{Key: "fallback.enabled", Kind: KindBool, Default: false})
	if got := f.Bool(context.Background(), "fallback.enabled", false); !got {
		t.Fatalf("Bool = %v, want true (env override)", got)
	}
}

func TestBoolInvalidEnvFailsFast(t *testing.T) {
	t.Parallel()
	env := map[string]string{"SCORECARD_FLAG_FALLBACK_ENABLED": "notabool"}
	_, err := New(fakeEnv(env), Config{Domain: t.Name()},
		Definition{Key: "fallback.enabled", Kind: KindBool, Default: false})
	if err == nil {
		t.Fatal("New: want error for invalid boolean, got nil")
	}
}

func TestStringEnvOverrideWithinAllowed(t *testing.T) {
	t.Parallel()
	env := map[string]string{"SCORECARD_FLAG_FALLBACK_MODE": "safety-net"}
	def := Definition{
		Key: "fallback.mode", Kind: KindString,
		Default: "fetch-first", Allowed: []string{"fetch-first", "safety-net"},
	}
	f := newForTest(t, env, def)
	if got := f.String(context.Background(), "fallback.mode", "fetch-first"); got != "safety-net" {
		t.Fatalf("String = %q, want %q", got, "safety-net")
	}
}

func TestStringDefaultWhenUnset(t *testing.T) {
	t.Parallel()
	def := Definition{
		Key: "fallback.mode", Kind: KindString,
		Default: "fetch-first", Allowed: []string{"fetch-first", "safety-net"},
	}
	f := newForTest(t, nil, def)
	if got := f.String(context.Background(), "fallback.mode", "safety-net"); got != "fetch-first" {
		t.Fatalf("String = %q, want %q (declared default)", got, "fetch-first")
	}
}

func TestStringDisallowedValueFailsFast(t *testing.T) {
	t.Parallel()
	env := map[string]string{"SCORECARD_FLAG_FALLBACK_MODE": "nonsense"}
	_, err := New(fakeEnv(env), Config{Domain: t.Name()}, Definition{
		Key: "fallback.mode", Kind: KindString,
		Default: "fetch-first", Allowed: []string{"fetch-first", "safety-net"},
	})
	if err == nil {
		t.Fatal("New: want error for disallowed value, got nil")
	}
}

func TestUnknownProviderFailsFast(t *testing.T) {
	t.Parallel()
	_, err := New(fakeEnv(nil), Config{Provider: "flagd", Domain: t.Name()})
	if err == nil {
		t.Fatal("New: want error for unknown provider, got nil")
	}
}

func TestFailSafeUnregisteredKeyReturnsDefault(t *testing.T) {
	t.Parallel()
	f := newForTest(t, nil) // no definitions registered
	if got := f.Bool(context.Background(), "not.registered", true); !got {
		t.Fatalf("Bool = %v, want true (fail-safe default)", got)
	}
	if got := f.String(context.Background(), "not.registered", "fallback"); got != "fallback" {
		t.Fatalf("String = %q, want %q (fail-safe default)", got, "fallback")
	}
}

func TestExplicitEnvNameOverride(t *testing.T) {
	t.Parallel()
	env := map[string]string{"CUSTOM_FLAG_VAR": "true"}
	f := newForTest(t, env, Definition{
		Key: "fallback.enabled", Env: "CUSTOM_FLAG_VAR", Kind: KindBool, Default: false,
	})
	if got := f.Bool(context.Background(), "fallback.enabled", false); !got {
		t.Fatalf("Bool = %v, want true (explicit env name)", got)
	}
}
