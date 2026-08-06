# Design: Feature flagging via OpenFeature

## Context

Configuration in this project is deliberately env-driven, validated at startup,
and fail-fast (design D10); the server also starts **offline** (no network I/O at
boot — see the deferred OSS-Fuzz client). The upstream-fallback work introduces
the project's first values that are genuinely *behavioral toggles* rather than
configuration, and that an operator may want to change without a redeploy. This
change adds a first-class way to express those, without abandoning the config
philosophy for everything else.

## Goals / Non-Goals

**Goals:**

- One vendor-neutral seam for runtime flags, swappable at the provider level.
- Preserve today's behavior with the default provider: env-driven, deterministic,
  offline startup, fail-fast on bad values.
- Make flag evaluation fail-safe: a backend problem must never break a request.
- Draw and document a clear line between flags and configuration.

**Non-Goals** (see proposal): dynamic/remote providers, targeting/experimentation,
moving configuration behind flags, and any flag-management UI.

## Decisions

### FF1 — OpenFeature as the abstraction (not a homegrown flag layer)

OpenFeature is a CNCF, vendor-neutral specification with a Go SDK. Depending on
the *spec/SDK* rather than a vendor means the backend is swappable and the project
avoids lock-in, while aligning with the broader ecosystem. A homegrown boolean
layer would be marginally smaller today but would have to be reinvented (and
migrated) the moment a dynamic backend is wanted.

### FF2 — Default provider: in-process, static/environment-backed

The default provider resolves flags from static/env values in-process. This
preserves D10: no network dependency, no boot-time call, deterministic, and
fail-fast on invalid values. It also keeps runtime attack surface minimal — a
non-trivial consideration for a security-posture tool. Dynamic providers
(`flagd`, hosted) are opt-in later through the same seam (FF5).

### FF3 — Flags vs. configuration is a project rule, not a case-by-case call

- **Configuration** — endpoints, credentials, timeouts, TTLs, concurrency,
  tokens, bucket/listen URLs. Sourced from the environment, validated at startup,
  fail-fast. Unchanged.
- **Flags** — runtime *behavioral* toggles and rollouts (on/off, mode selection).
  Read through the `internal/flags` seam, always with a safe in-code default.

The test: if you would ever want to change it *without a redeploy* (incident
kill-switch, gradual rollout, mode shift), it is a flag; if it is a value the
process needs to boot and connect, it is configuration. This rule is documented
in AGENTS.md so future changes classify new settings consistently.

### FF4 — Fail-safe defaults are mandatory

Every flag has an in-code default supplied at the call site. An evaluation error
(misconfiguration, or a future remote provider being unreachable) returns that
default; it never propagates as a request failure. This keeps the flag system
from becoming a new availability dependency.

### FF5 — A single in-app seam; the app never imports the SDK directly

Call sites depend on `internal/flags` (typed accessors like `Bool(ctx, key,
default)` / `String(ctx, key, default)`), not on the OpenFeature SDK. This makes
flags trivially fakeable in tests, localizes any SDK/provider change to one
package, and is precisely what lets `add-upstream-fallback` read
`fallback.enabled` / `fallback.mode` without knowing where the values come from.

### FF6 — Flag key and type conventions

Flag keys are lower-case, dot-scoped by capability (`fallback.enabled`,
`fallback.mode`). Booleans for on/off; strings for enumerations (validated by the
consumer against its allowed set, with a safe default). Keys and their defaults
are registered/documented alongside the consuming capability, not centralized, so
a capability owns its own flags.

### FF7 — Startup validation applies to the static provider

Because the default provider is static/env, known flag values can be validated at
startup and fail fast, keeping the D10 property for env-sourced flags. This
guarantee is explicitly scoped to the static provider: a future dynamic provider
resolves values at runtime, so its values cannot be startup-validated — consumers
rely on FF4 (safe defaults) and their own value validation instead. Documenting
this boundary now avoids a false expectation later.

## Reconciliation with existing config (D10)

This change does not move any existing configuration behind flags. It adds a
parallel, narrow mechanism for behavioral toggles and writes down when to use
which. The only new configuration value is `SCORECARD_FLAGS_PROVIDER` (default
`static`), which is itself ordinary startup configuration.

## Upstream graft

Like the fallback tier, feature flagging is **incubator/deployment concern, not a
graft target**: `scorecard serve` and `scorecard-webapp` would not inherit it.
It lives behind `internal/flags` and the config seam specifically so it does not
entangle the durable, graftable core paths (design D11; `docs/upstream-graft.md`).

## Risks / trade-offs

- **A dependency added to a security tool.** Mitigated by keeping the default
  provider in-process (no network client) and the seam thin; the dependency
  surface is the SDK plus one provider.
- **Two config paradigms (env + flags).** Mitigated by FF3's explicit rule and
  documentation, and by keeping the flag set deliberately tiny.
- **Abstraction for ~1–2 flags today.** Accepted: adopting while small is far
  cheaper than retrofitting, and fallback is a real first consumer, not a
  hypothetical one.
