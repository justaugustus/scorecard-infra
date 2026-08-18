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

// Package flags is the single seam for runtime feature flags (design FF1/FF5).
// The rest of the app evaluates flags through this package, never through the
// OpenFeature SDK directly, so the provider is swappable and flags are trivially
// fakeable in tests.
//
// The default provider is in-process and seeded from the environment at startup
// (FF2): it performs no network I/O, preserving the project's offline, fail-fast,
// 12-factor startup (design D10). A capability declares its flags as Definitions
// and passes them to New; New reads each flag's environment override, validates
// it, and fails fast on an invalid static value (FF7). Evaluation is fail-safe:
// any error returns the caller's in-code default rather than failing the request
// (FF4).
//
// This is deliberately not a place for configuration. Endpoints, credentials,
// timeouts, and tuning stay environment configuration; only runtime behavioral
// toggles belong here (design FF3).
package flags

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/memprovider"
)

// ProviderStatic is the default provider: an in-process store seeded from the
// environment. It is the only provider supported today; dynamic providers
// (e.g. flagd) are a future addition selected by the same setting.
const ProviderStatic = "static"

// defaultDomain is the OpenFeature domain used in production. Tests override it
// (Config.Domain) so each instance binds to its own provider without colliding
// on the global registry.
const defaultDomain = "scorecard-api"

// envPrefix namespaces the environment overrides for flags, keeping them
// visually distinct from ordinary configuration (design FF3).
const envPrefix = "SCORECARD_FLAG_"

var (
	errUnknownProvider = errors.New("flags: unknown provider")
	errInvalidValue    = errors.New("flags: invalid flag value")
	errUnknownKind     = errors.New("flags: unknown flag kind")
)

// Kind is the value type of a flag.
type Kind int

const (
	// KindBool is a boolean on/off toggle.
	KindBool Kind = iota
	// KindString is a string, optionally constrained to an allowed set.
	KindString
)

// Definition declares a single flag a capability owns (design FF6). It is passed
// to New, which reads the flag's environment override, validates it, and seeds
// the provider. Field order favors pointer packing (govet fieldalignment).
type Definition struct {
	// Default is the value used when no environment override is set. Its dynamic
	// type MUST match Kind (bool for KindBool, string for KindString).
	Default any
	// Key is the dot-scoped flag key, e.g. "fallback.enabled".
	Key string
	// Env overrides the environment variable name; empty derives it from Key
	// (e.g. "fallback.enabled" -> "SCORECARD_FLAG_FALLBACK_ENABLED").
	Env string
	// Allowed optionally constrains a KindString flag to a set of values; an
	// override or default outside the set fails validation at startup.
	Allowed []string
	// Kind is the flag's value type.
	Kind Kind
}

// Config selects the provider and (for tests) the OpenFeature domain.
type Config struct {
	// Provider selects the flag backend; empty defaults to ProviderStatic.
	Provider string
	// Domain overrides the OpenFeature domain; empty uses the production default.
	Domain string
}

// Flags evaluates feature flags through the configured provider.
type Flags struct {
	client *openfeature.Client
}

// New builds a Flags over the selected provider, seeding it from the given
// definitions and their environment overrides. It returns an error on an unknown
// provider or an invalid static flag value (design FF7), so the server fails fast
// at startup rather than serving with a misconfigured flag.
func New(getenv func(string) string, cfg Config, defs ...Definition) (*Flags, error) {
	provider := cfg.Provider
	if provider == "" {
		provider = ProviderStatic
	}
	if provider != ProviderStatic {
		return nil, fmt.Errorf("%w: %q (only %q is supported)", errUnknownProvider, provider, ProviderStatic)
	}

	seeded, err := seed(getenv, defs)
	if err != nil {
		return nil, err
	}

	domain := cfg.Domain
	if domain == "" {
		domain = defaultDomain
	}
	if serr := openfeature.SetNamedProviderAndWait(domain, memprovider.NewInMemoryProvider(seeded)); serr != nil {
		return nil, fmt.Errorf("flags: setting provider: %w", serr)
	}
	return &Flags{client: openfeature.NewClient(domain)}, nil
}

// seed resolves each definition's effective value from its environment override
// (or its default) and builds the in-memory flag set, failing on an invalid value.
func seed(getenv func(string) string, defs []Definition) (map[string]memprovider.InMemoryFlag, error) {
	out := make(map[string]memprovider.InMemoryFlag, len(defs))
	for i := range defs {
		d := &defs[i]
		value, err := resolve(getenv, d)
		if err != nil {
			return nil, err
		}
		out[d.Key] = memprovider.InMemoryFlag{
			Key:            d.Key,
			State:          memprovider.Enabled,
			DefaultVariant: "default",
			Variants:       map[string]any{"default": value},
		}
	}
	return out, nil
}

// resolve computes a definition's effective value: the parsed environment
// override when set, otherwise the declared default. Both paths are validated.
func resolve(getenv func(string) string, d *Definition) (any, error) {
	raw := getenv(envName(d))
	switch d.Kind {
	case KindBool:
		if raw == "" {
			b, ok := d.Default.(bool)
			if !ok {
				return nil, fmt.Errorf("%w: default for %q is not a bool", errInvalidValue, d.Key)
			}
			return b, nil
		}
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %s=%q is not a boolean", errInvalidValue, envName(d), raw)
		}
		return v, nil
	case KindString:
		v, ok := d.Default.(string)
		if !ok {
			return nil, fmt.Errorf("%w: default for %q is not a string", errInvalidValue, d.Key)
		}
		if raw != "" {
			v = raw
		}
		if err := validateAllowed(d, v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, fmt.Errorf("%w: %d for %q", errUnknownKind, d.Kind, d.Key)
	}
}

// validateAllowed enforces a KindString flag's allowed set, if any.
func validateAllowed(d *Definition, v string) error {
	if len(d.Allowed) == 0 {
		return nil
	}
	for _, a := range d.Allowed {
		if v == a {
			return nil
		}
	}
	return fmt.Errorf("%w: %s=%q is not one of %v", errInvalidValue, envName(d), v, d.Allowed)
}

// envName returns the environment variable backing a flag: its explicit Env, or
// the prefixed, upper-cased key with separators normalized to underscores.
func envName(d *Definition) string {
	if d.Env != "" {
		return d.Env
	}
	k := strings.NewReplacer(".", "_", "-", "_").Replace(d.Key)
	return envPrefix + strings.ToUpper(k)
}

// Bool evaluates a boolean flag, returning def on any evaluation error (FF4).
func (f *Flags) Bool(ctx context.Context, key string, def bool) bool {
	v, err := f.client.BooleanValue(ctx, key, def, openfeature.EvaluationContext{})
	if err != nil {
		return def
	}
	return v
}

// String evaluates a string flag, returning def on any evaluation error (FF4).
func (f *Flags) String(ctx context.Context, key, def string) string {
	v, err := f.client.StringValue(ctx, key, def, openfeature.EvaluationContext{})
	if err != nil {
		return def
	}
	return v
}
