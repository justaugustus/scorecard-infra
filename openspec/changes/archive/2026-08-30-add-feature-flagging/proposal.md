# Proposal: Feature flagging via OpenFeature

## Why

The project is about to introduce its first runtime behavioral toggles — starting
with the upstream fallback (whether it is active, and which ordering mode it
uses). These are not configuration. Configuration is endpoints, credentials,
timeouts, and tuning: set at deploy, validated at startup, fail-fast (design
D10). A behavioral toggle is something an operator may want to flip *without a
redeploy* — for example, disabling the fallback during an upstream incident, or
shifting its mode under load.

Rather than grow ad-hoc booleans and later retrofit them, adopt a single,
vendor-neutral flag abstraction now, while the surface is one or two flags and
the cost is small. **OpenFeature** (a CNCF project) is that abstraction: the SDK
is provider-agnostic, so adopting it does not lock the project to any flag
backend. Starting with an in-process static/environment-backed provider keeps
today's 12-factor, fail-fast, **offline** startup behavior intact, while leaving
a clean path to a dynamic provider (`flagd`, or a hosted service) if a dynamic or
multi-tenant deployment ever emerges.

This change lands the mechanism and the convention. The upstream-fallback change
(`add-upstream-fallback`) is its first consumer and validates it.

## What Changes

- Add the **OpenFeature Go SDK** and a single evaluation seam (`internal/flags`)
  that the rest of the app uses to read typed flag values, each with an in-code
  default.
- Ship a minimal **in-process static/environment provider** as the default, so no
  network I/O or external dependency is introduced at startup.
- Make the provider **selectable by configuration** (default = static/env), so a
  dynamic provider can be introduced later without touching call sites.
- Guarantee **fail-safe evaluation**: a flag that cannot be evaluated returns its
  coded default and never fails a request.
- Establish the **flags-vs-configuration convention** as a project rule and
  document it (AGENTS.md / README): configuration is env-driven and
  startup-validated; flags are runtime behavioral toggles read through the seam.

## Capabilities

### New Capabilities

- `feature-flags`: runtime feature-flag evaluation via OpenFeature through a
  single in-app seam, with a pluggable provider (default in-process static/env),
  fail-safe defaults, preserved offline startup, and a documented
  flags-vs-configuration policy.

### Modified Capabilities

<!-- None. The upstream fallback consumes this capability in its own change. -->

## Impact

- **New code:** `internal/flags` — OpenFeature client bootstrap, provider
  selection, and typed accessors (bool/string) that the app depends on instead of
  the SDK directly (testable with a fake).
- **Dependencies:** `github.com/open-feature/go-sdk` plus the in-process provider.
  A security-posture tool should keep this minimal; the default provider adds no
  network client.
- **Config:** `SCORECARD_FLAGS_PROVIDER` (default `static`), and provider-specific
  settings only if a non-default provider is selected.
- **Startup:** unchanged with the default provider — no network call, still
  offline-capable, still fail-fast on invalid static flag values.
- **Consumers:** `add-upstream-fallback` (`fallback.enabled`, `fallback.mode`) is
  the first consumer.
- **Compatibility:** additive; with the default provider, behavior is equivalent
  to reading the same values from the environment today.

## Non-goals

- **Dynamic/remote providers** (`flagd`, LaunchDarkly, hosted) are not wired in
  v1 — only the seam and the default in-process provider. Adding one is a later
  change.
- **Per-request targeting, segmentation, or experimentation/analytics** beyond
  OpenFeature's built-in evaluation context are out of scope.
- **Moving configuration behind flags.** Endpoints, credentials, timeouts, TTLs,
  concurrency, and tokens remain environment configuration per the convention.
- **A flag-management UI or audit pipeline.**
