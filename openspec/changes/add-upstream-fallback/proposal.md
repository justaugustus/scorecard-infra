# Proposal: Upstream result fallback (api.scorecard.dev)

## Why

A live scan is this server's most expensive path — minutes of wall-clock, SCM
token budget, and rate-limit pressure — and for many repositories it is not even
the most complete one. When the operator does **not** own the target repository
(the common "score my dependencies" case), the configured token cannot read
org/admin-scoped signals such as branch protection, so those checks come back
inconclusive (`-1`). The public OpenSSF result on `api.scorecard.dev` for that
same repository is often as complete or more complete, and far cheaper to obtain.

So the upstream is not "weaker" — it is a different point on a cost / freshness /
coverage trade-off: cheaper and sometimes more complete, but bounded-stale,
`latest`-only, and limited to repositories that opted in via `publish_results`.
This change lets the orchestrator use it as an additional tier, with honest,
per-result provenance so a client always knows what it received.

## What Changes

- Add an optional **upstream fallback** tier to the orchestrator. It is
  configured by `SCORECARD_FALLBACK_URL` (the upstream endpoint) and gated at
  runtime by the **`fallback.enabled`** feature flag (default on when a URL is
  set), so it can be killed without a redeploy. It reuses this server's own
  `/projects` GET contract, since `api.scorecard.dev` speaks it.
- Choose ordering with the **`fallback.mode`** feature flag, default
  **`fetch-first`**:
  - `fetch-first` — cache → upstream → live scan on an upstream miss. The
    efficiency path: reuse a recent upstream result, scan only when there is none.
  - `safety-net` — cache → live scan → upstream only when the scan cannot produce
    a result (skipped/errored/rate-limited). The "always scan my own repos, fall
    back only when I can't" path.
- Bound upstream staleness with **`SCORECARD_FALLBACK_MAX_AGE`** (default 7d,
  aligned with the weekly public cron): an upstream result older than max-age is
  not used, and the request falls through to a scan.
- **Backfill** a used upstream result (within max-age) into the store, tagged as
  upstream, and make freshness **source-aware**: locally-scanned `latest` results
  age by `SCORECARD_LATEST_TTL`; upstream-sourced ones age by max-age. This warms
  the cache without a stale weak result pinning it.
- Report an honest **`upstream` provenance source** and a **uniform, source-
  agnostic completeness** rule: any result — live, cached, or upstream — is
  complete only if every check this server would run is present and conclusive.
- **Commit-pinned requests bypass** the upstream (it answers only `latest`).
- **`/capabilities`** advertises the fallback mode and its caveats when enabled.

## Capabilities

### New Capabilities

- `upstream-fallback`: an optional, best-effort, read-only client that fetches an
  existing `latest` result from a configured upstream Scorecard API over the
  `/projects` contract; its enablement (URL + `fallback.enabled`), ordering modes
  (`fallback.mode`, default `fetch-first`), max-age bound, provenance, and
  commit-pinned bypass.

### Modified Capabilities

- `result-cache`: the read-through seam gains the fallback tier (positioned by
  mode), source-aware freshness, upstream backfill, and true-source reporting.
- `result-store`: results are stored and retrieved with an origin tag (locally
  scanned vs. upstream) without altering the canonical JSON2 body.
- `api-server`: `/capabilities` and the source header advertise the fallback and
  the `upstream` source.

## Impact

- **Depends on** `add-feature-flagging`: `fallback.enabled` / `fallback.mode` are
  feature flags read through `internal/flags`; the orchestrator captures the flag
  client that `main` initializes.
- **New code:** `internal/fallback` (a `Fallback` interface + `/projects` HTTP
  client). `internal/orchestrator` gains mode-aware ordering, source-aware
  freshness, and backfill; `internal/store` gains an origin tag (blob metadata);
  `internal/model` gains `SourceUpstream` and the uniform completeness helper;
  `internal/config` gains the URL, timeout, and max-age; `internal/httpapi`
  advertises the mode/caveats.
- **Config:** `SCORECARD_FALLBACK_URL`, `SCORECARD_FALLBACK_TIMEOUT`,
  `SCORECARD_FALLBACK_MAX_AGE`. **Flags:** `fallback.enabled`, `fallback.mode`.
- **External systems:** an outbound dependency on the configured upstream, only
  when enabled, best-effort, short-timeout; no upstream call at startup.
- **Compatibility:** additive and off by default (no `SCORECARD_FALLBACK_URL` →
  unchanged behavior). Existing store entries without an origin tag are treated as
  locally scanned.

## Non-goals

- **Non-`latest` (commit-pinned) upstream lookups.** The public upstream cannot
  answer them; commit-pinned requests scan.
- **Upstream rescue on the synchronous-timeout (202) path.** In `safety-net`, v1
  rescues only terminal scan outcomes (skip/error); a still-running scan keeps
  today's 202. Serving upstream on 202 is a documented future option (design F9).
- **Writing/publishing results upstream** (the signed-upload `POST` path) —
  remains deferred.
- **Credentialed/private upstreams.** v1 assumes a plain public GET.
- **Message broker / distributed workers, warm-cache scheduler, and
  analytics/index layer** remain deferred to later changes.
