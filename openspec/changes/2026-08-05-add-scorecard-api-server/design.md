# Design: Scorecard API Server (cloud-agnostic, hybrid cache + live)

## Context

OpenSSF Scorecard produces structured security-posture results, but the only hosted
API (`api.scorecard.dev`) covers opted-in public repos and runs on a GCP-specific
stack (GCS + BigQuery + PubSub + Cloud Endpoints). This change adds a **self-hostable,
cloud-agnostic** server that (a) serves cached results from any object store and
(b) generates them live on demand — a **read-through cache over the scan engine**.

Key facts established during discovery (from the `ossf/scorecard`,
`ossf/scorecard-webapp`, and `scorecard-mcp` codebases):

- **The webapp read path is already cloud-abstracted.** `scorecard-webapp`'s
  `app/server/get_results.go` reads results via `gocloud.dev/blob`; the only GCP
  lock-ins are a blank-imported `gcsblob` driver and a hardcoded
  `gs://ossf-scorecard-results` constant. Portability is parameterization, not a
  rewrite.
- **The blob key contract is fixed and simple.** `{host}/{org}/{repo}/results.json`
  for latest; `{host}/{org}/{repo}/{commit}/results.json` for a pinned commit. Body
  is canonical Scorecard **JSON2**. Matching this makes any producer's output
  directly servable.
- **The read path needs no BigQuery.** BigQuery lives only in `scorecard/cron`
  (`cron/internal/bq`) for batch/analytics. Per-repo `GET` just reads one blob.
- **`pkg/scorecard.Run` imports out-of-tree** (Go `internal/` only blocks direct
  imports of Scorecard's `internal/…`; the public `pkg/scorecard` compiles fine),
  so live scanning does not require living in-tree.
- **There is already an in-tree HTTP server: `scorecard serve`.** It is on-demand
  (computes fresh per request via `pkg/scorecard.Run`), uses plain `net/http` with
  `GET /?repo=…&commit=…&show_details=…`, has **no store** and **no cloud
  dependency**, and exposes `/` + `/health`. It is the closest existing surface to
  our live path — but it does **not** speak the `/projects/…` webapp contract, and
  it has a history of under-maintenance (issue #4627: null details/no flags; PR
  #4665: a proposed chi/REST refactor whose merge status is unconfirmed — `main`
  still uses `net/http`).
- **`scorecard-mcp` is the reference client.** Its `CachedRESTProvider` GETs
  `{base}/projects/{platform}/{org}/{repo}[?commit=]` and its caveats/capabilities
  are hardcoded to the public cache — pointing it at another backend would misreport
  provenance. This motivates a server-advertised `/capabilities` endpoint.

## Goals / Non-Goals

**Goals:**

- Serve the `scorecard-webapp` GET contract from any object store; be a drop-in
  `--base-url` target for `scorecard-mcp`.
- Generate results live on cache miss/stale, persist them, and return them.
- Zero cloud lock-in: storage/messaging/config are provider-neutral and env-driven.
- Emit deterministic, provenance-tagged JSON2 so results are cache-keyable per
  repo+commit.
- Structure code so the durable pieces graft upstream (webapp contract/blob;
  `scorecard serve`).

**Non-Goals** (see proposal for the full list): message broker/distributed workers,
warm-cache scheduler, analytics/index layer, signed-upload POST, request-level
auth/multi-tenancy, reimplementing scoring, and any internal deployment glue.

## Decisions

### D1 — Language & stack: Go, importing `pkg/scorecard`

Go matches the Scorecard ecosystem and lets us import `pkg/scorecard.Run` and
`docs/checks` directly. HTTP via the standard library first (chi optional if routing
grows). _Alternative:_ a non-Go rewrite — rejected (loses the engine import and the
upstream graft path).

### D2 — The read-through cache (orchestrator) is the central seam

All requests flow through one orchestrator that decides serve-vs-scan:

```text
GetOrProduce(ref, commit):
  key   := storeKey(ref, commit)         # {host}/{org}/{repo}[/{commit}]/results.json
  hit   := store.Get(key)
  if hit != nil and fresh(hit, commit):  # commit-pinned => always fresh
      return hit (source=cached)
  return singleflight(key, func() {      # coalesce concurrent identical requests
      res := scan.Run(ref, commit)       # live
      store.Put(key, res)                # populate cache (+ latest pointer)
      return res (source=live)
  })
```

The orchestrator owns freshness policy, single-flight, and the sync/async decision
(D5). It depends on two interfaces — a `Store` and a `Scanner` — so both backends are
swappable and independently testable. _Alternative:_ wire handlers straight to the
store or the engine — rejected (couples the HTTP layer to a single mode; the hybrid
behavior is the whole point).

### D3 — Cloud-agnostic storage via `gocloud.dev/blob`

`internal/store` opens a bucket from a URL env var (e.g.
`SCORECARD_RESULTS_BUCKET_URL`) and blank-imports every driver: `s3blob`
(AWS S3, MinIO, Ceph, any S3-compatible), `azureblob`, `gcsblob`, `fileblob`
(local dev), `memblob` (tests). Credentials come from each backend's default chain,
which `gocloud.dev` honors. **No hardcoded `gs://`; no BigQuery.** _Alternative:_
per-cloud SDKs behind our own interface — rejected (reinvents the CDK the webapp
already relies on).

### D4 — Blob key + body contract (match the webapp exactly)

Keys: `{host}/{org}/{repo}/results.json` (latest), `{host}/{org}/{repo}/{commit}/results.json`
(pinned). Body: canonical Scorecard **JSON2** (`{date, repo{name,commit}, scorecard{version,commit},
score, checks[], metadata}`). Matching this means the same objects are servable by
`scorecard-webapp` and readable by `scorecard-mcp` unchanged, and any conforming
producer can populate the cache.

### D5 — Freshness policy and sync-vs-async responses

- **Commit-pinned** (`?commit=SHA`) results are **immutable** → cache forever; a hit
  is always fresh.
- **`latest`** results carry a **TTL** (config, e.g. `SCORECARD_LATEST_TTL`); a stale
  hit triggers a refresh scan.
- **Response mode:** attempt a **synchronous** scan within a request timeout and
  return `200` with the result; if it would exceed the timeout (or async mode is
  configured), return `202` with a `Retry-After` so the client (e.g. the MCP)
  re-requests. Immutability of commit results makes retries safe.

### D6 — Single-flight de-duplication

Concurrent requests for the same key MUST coalesce into **one** scan (`sync.Map` +
`golang.org/x/sync/singleflight` or equivalent). Live scans are expensive and
rate-limited; without coalescing, a burst of MCP calls for one repo would trigger N
redundant scans and exhaust SCM rate limits.

### D7 — `/capabilities`: server-advertised caveats (fixes the MCP gap)

`scorecard-mcp` hardcodes public-cache caveats (`internal/provider/rest.go`:
opted-in-only, weekly scan omits three checks). Those are false for this server
(it scans on demand, all checks, any accessible repo). The server therefore exposes
`GET /capabilities` returning: source/mode (`cached+live`), the check set it runs,
whether coverage requires opt-in (no), freshness policy, and any caveats. Clients
report provenance from this rather than assuming. _Follow-up (separate repo):_ teach
`scorecard-mcp` to read `/capabilities` instead of hardcoding.

### D8 — Live scan engine + token/rate manager

`internal/scan` wraps `pkg/scorecard.Run` following the upstream `ScorecardWorker`
pattern: create the GitHub/GitLab, OSS-Fuzz, CII, and vulnerabilities clients **once**
and reuse them across scans; format to JSON2; write back to the store. `internal/tokens`
provides an SCM **token pool** (GitHub App installation tokens or a PAT pool), a
**per-host rate limiter**, and backoff/retry. SCM API rate limits — not CPU — are the
scaling bottleneck; a single token is unsafe across concurrent scans.

### D9 — Concurrency: in-process worker pool now, broker later

v0 bounds live scans with an in-process worker pool (the `oss-pulse` pattern) plus
D6 single-flight. A `gocloud.dev/pubsub` broker (SQS/Service Bus/Kafka/NATS) for
multi-node fan-out is deferred (proposal Non-goals) and slots in behind the same
`Scanner` seam.

### D10 — Configuration: 12-factor, all env

Bucket URL, `latest` TTL, request/scan timeouts, enabled checks, worker concurrency,
listen port, and SCM credentials all come from environment. Nothing cloud-specific is
compiled in. Missing required config (e.g. bucket URL) fails fast at startup.

### D11 — Upstream graft map + `scorecard serve` reconciliation

Structure code so durable pieces move upstream cleanly:

- **Contract handlers + blob read path → `ossf/scorecard-webapp`** (a small
  bucket-URL parameterization + driver imports; benefits the public server too).
- **Live scan + HTTP surface → `ossf/scorecard` `scorecard serve`.** Endgame: teach
  `serve` the `/projects/…` contract and an optional blob cache, so the ecosystem has
  one HTTP server, not two divergent ones.

**Open reconciliation question (resolve before implementing the HTTP layer):** do we
reuse/extend `scorecard serve`'s handler wiring here, or implement fresh and
back-port? First **verify PR #4665's true merge status** (main still uses `net/http`,
so treat its chi/REST refactor as unlanded).

### D12 — Responsible-AI framing

Every response declares `source` (`cached` vs `live`), freshness, and completeness.
The server never asserts a repo "is secure/insecure." Framing derives from
Scorecard's documented non-goals (heuristics with false positives/negatives;
aggregate scores say nothing about individual behaviors).

## Risks / Trade-offs

- **SCM rate limits throttle live scans** → token pool + per-host limiter + backoff
  (D8); single-flight (D6); TTL + warm-cache (deferred) reduce live volume.
- **Live scan latency vs. request timeout** → sync-with-timeout then `202`/retry
  (D5); commit-immutability makes retries cheap.
- **`gocloud.dev` driver/credential differences across clouds** → integration-test
  against `fileblob` and MinIO in CI; document per-backend URL + cred setup.
- **Scorecard v5 API drift** → pin the module; keep `scan` a thin adapter over
  `pkg/scorecard`.
- **Divergence from `scorecard serve`** → treat serve reconciliation as a first-class
  design question (D11) so we don't create a third HTTP server permanently.
- **`-1` misread as failing** → pass scores through unchanged; label `-1` as
  inconclusive.
- **Serving stale `latest`** → explicit TTL + `date` in every result; clients can
  pin a commit for immutability.

## Migration Plan

Greenfield — no data migration. Phased:

1. **This change (v0):** `store` (blob, all drivers), `orchestrator` (read-through
   cache + single-flight + sync/async), `scan` (`pkg/scorecard.Run` via in-proc
   pool), `tokens` (pool + limiter), HTTP contract (`/projects`, `/badge`,
   `/capabilities`, `/health`), verified against `fileblob` + MinIO with
   `scorecard-mcp` as the acceptance test.
2. **Later:** `gocloud.dev/pubsub` broker; warm-cache scheduler; analytics/index.
3. **Later:** graft upstream — webapp bucket parameterization; `scorecard serve`
   `/projects` + blob cache.

## Open Questions

- Verify PR #4665 merge status and decide the `scorecard serve` reuse/extend approach
  (D11).
- Whether to import `scorecard-webapp`'s generated models directly or vendor
  `openapi.yaml` and regenerate.
- Default `latest` TTL and default request/scan timeout values.
- `/capabilities` payload shape (align with a future `scorecard-mcp` reader).
- Badge rendering: reuse the webapp renderer vs. minimal in-repo implementation.
- Canonical result host header/metadata to record as the `source_url` for attribution.
