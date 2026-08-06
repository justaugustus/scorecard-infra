# Design: Upstream result fallback (api.scorecard.dev)

## Context

The base server is a read-through cache over an in-process scan engine, with one
serve-vs-produce seam in `orchestrator.GetOrProduce`. A live scan is the
expensive path, and it is the only fallback when the store misses.

`api.scorecard.dev` is effectively a remote, read-only cache tier that already
speaks this server's `/projects` GET contract. Critically, it is **not strictly
weaker** than a local scan:

- For repositories the operator does **not** own, a local scan is token-limited —
  branch protection and other admin-scoped checks return inconclusive (`-1`) — so
  the upstream result is often as complete or more complete, and much cheaper.
- The upstream's real limits are different axes: it is bounded-stale (weekly
  cron), `latest`-only, and covers only repositories that opted in.

So the design treats the upstream as a first-class tier chosen for **cost and
freshness**, with per-result provenance and completeness reported honestly rather
than assuming "live is best."

## Goals / Non-Goals

**Goals:** insert the tier without disturbing behavior when disabled; default to
the efficiency path (`fetch-first`) since the upstream is often comparable;
report honest, source-agnostic provenance/completeness; warm the cache via
bounded backfill; reuse the `/projects` contract, `model.Parse`, provenance
headers, and `/capabilities`.

**Non-Goals** (see proposal): commit-pinned upstream lookups, 202-path rescue,
publishing upstream, credentialed upstreams.

## Decisions

### F1 — Fallback is a first-class orchestrator tier

A `Fallback` interface alongside `Scanner`, injected via a `WithFallback` option
(nil = disabled). Not a second `Store` (no `Put`, non-fatal, different freshness)
and not a `Scanner` (a GET is not a scan; conflating misreports provenance and
wrongly uses the 202/singleflight path). Returns `*scan.Result` to reuse
provenance derivation.

```go
type Fallback interface {
    Fetch(ctx context.Context, ref model.RepoRef) (*scan.Result, error)
}
```

### F2 — Enablement splits config from flags (depends on add-feature-flagging)

The upstream *endpoint* is configuration (`SCORECARD_FALLBACK_URL`,
`SCORECARD_FALLBACK_TIMEOUT`, `SCORECARD_FALLBACK_MAX_AGE`): set at deploy,
validated at startup. Whether the tier is *active* and *which ordering* it uses
are runtime behavioral toggles — feature flags `fallback.enabled` and
`fallback.mode`, read through `internal/flags` (design FF3). This is why
feature-flagging landed first: the orchestrator captures the flag client and
evaluates these per request, so a dynamic provider could later flip them without
a redeploy.

Effective enablement = a URL is configured **and** `fallback.enabled` (default
true) is on. A URL with the flag off is a clean kill-switch.

### F3 — `fetch-first` is the default ordering

- **`fetch-first`** (default): cache → upstream → scan on miss. Delivers the
  efficiency motivation; the upstream is often comparable and far cheaper, and for
  private/owned repos it simply misses and falls through to a scan.
- **`safety-net`**: cache → scan → upstream on a terminal scan failure. For
  operators who always want a full scan of their own repos and only want the
  upstream as a resilience net.

The default is `fetch-first` because the "weaker upstream" premise does not hold
(see Context); the operator picks `safety-net` when they specifically want
scan-first behavior.

### F4 — Uniform, source-agnostic completeness

Completeness is a property of a result, not of its source. A result — live,
cached, or upstream — is complete only if every check this server would run is
**present and conclusive** (no omitted checks, no `-1`). This replaces the earlier
upstream-only penalty: a token-limited local scan of a non-owned repo is reported
incomplete for the same reason an upstream result with omitted checks is.

### F5 — Synchronous fast GET, outside the singleflight/202 machinery

A `Fetch` is a sub-second GET with its own short timeout; it does not enter the
`singleflight`/`SyncTimeout`→202 path (which exists to coalesce and background
expensive scans). A miss or any error is non-fatal — log at debug, continue with
the active mode's next step.

### F6 — Bounded backfill with a store origin tag and source-aware freshness

A used upstream result within max-age is **backfilled** into the store so it warms
the cache. To keep provenance honest on later reads, the store records an
**origin tag** (locally scanned vs. upstream) as blob metadata, leaving the
canonical JSON2 body untouched. Freshness then becomes source-aware: a
locally-scanned `latest` entry ages by `SCORECARD_LATEST_TTL`; an upstream entry
ages by `SCORECARD_FALLBACK_MAX_AGE` (against the upstream result's own date).
When an upstream entry exceeds max-age it is re-evaluated (refetch/scan) rather
than served stale. A store hit reports its true source from the tag: `cached` for
local origin, `upstream` for upstream origin. Entries with no tag (legacy) are
treated as local.

This reverses the initial "no backfill" stance: with the origin tag the fidelity
concern (a weak result later served as `cached`) is resolved, and max-age bounds
the staleness — exactly the knob that makes backfill worthwhile.

### F7 — Commit-pinned requests bypass the fallback

The upstream answers only `latest`. When `commit != ""`, both modes skip the
upstream and scan (a scan can resolve an arbitrary commit).

### F8 — The fallback client reuses this server's own `/projects` contract

`GET {fallback_url}/projects/{host}/{org}/{repo}`, parsed with `model.Parse`. A
404/empty is a miss (`ErrFallbackMiss`); other non-2xx/transport errors are
non-fatal. A second scorecard-api instance can thus serve as another's upstream.

### F9 — `safety-net` v1 rescues terminal scan outcomes only

The upstream is consulted on `scan.ErrSkipped` or a scan error. A scan that
merely exceeds `SyncTimeout` keeps today's 202 while it finishes in the
background; diverting that to upstream would entangle the singleflight/202
semantics. "Serve upstream on 202" is a future option.

## Upstream graft

Like feature flagging, the fallback tier is incubator/deployment-specific and
**not a graft target**: the OpenSSF stack is itself the upstream cache. It is
isolated in `internal/fallback` and behind config/flags so it does not complicate
a future graft of the core paths (base design D11; `docs/upstream-graft.md`).

## Risks / trade-offs

- **Serving an occasionally-staler result in `fetch-first`.** Bounded by max-age
  and reported via honest provenance/completeness; `safety-net` is available for
  scan-first operators.
- **Outbound dependency / latency.** Short timeout, non-fatal fall-through,
  disabled by default, no startup call.
- **Store metadata coupling.** The origin tag rides on blob metadata, which every
  supported backend supports; legacy untagged entries degrade to "local".
