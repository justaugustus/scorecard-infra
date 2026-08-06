# Upstream graft map & `scorecard serve` reconciliation

This repo is an **incubator, not a permanent fork**. It exists to prove out a
cloud-agnostic, hybrid (cached + live) Scorecard results API, structured so the
durable pieces graft back into the OpenSSF ecosystem cleanly rather than
becoming a third, divergent HTTP server. This note records **where each piece is
meant to land** and the **current reconciliation status** with the existing
in-tree server, `scorecard serve`.

It is the detailed companion to design decision **D11** (and touches **D4**,
**D7**, **D13**); the README and `AGENTS.md` point here. See
`openspec/changes/archive/2026-08-06-add-scorecard-api-server/design.md`.

## Graft targets

There are three homes for code in play:

- **`ossf/scorecard-webapp`** — owns the GET contract and the blob read path
  (`app/server/get_results.go`). Already cloud-abstracted via `gocloud.dev/blob`;
  its only GCP lock-ins are a blank-imported `gcsblob` driver and a hardcoded
  `gs://ossf-scorecard-results` constant.
- **`ossf/scorecard`** — owns the scan engine (`pkg/scorecard.Run`, `docs/checks`)
  and the in-tree HTTP server `scorecard serve`.
- **This repo (`uwu-tools/scorecard-api`)** — the incubator. Holds the novel
  glue (the read-through cache, token/rate management, `/capabilities`) plus
  thin adapters that mirror upstream so they can move.

## Component graft map

| Package         | Role                                              | Graft target                                | What the graft entails |
|-----------------|---------------------------------------------------|---------------------------------------------|------------------------|
| `internal/store`   | `gocloud.dev/blob` store; backend-by-URL; the `{host}/{org}/{repo}[/{commit}]/results.json` key contract | **`scorecard-webapp`** (read path) | Parameterize the bucket URL (drop the hardcoded `gs://`); blank-import the non-GCS drivers. A small change that benefits the public server too. |
| `internal/httpapi` — `/projects`, `/badge` | The webapp GET contract | **`scorecard-webapp`** | The contract already lives there; our handlers mirror it. Grafting is mostly reconciling routing/serialization, not new behavior. |
| `internal/scan`    | Wraps `pkg/scorecard.Run`; reuses OSS-Fuzz/CII/vuln clients across scans, per-scan repo client (mirrors the cron `ScorecardWorker`) | **`ossf/scorecard`** (`scorecard serve`) | Teach `serve` to persist + reuse results instead of always computing fresh. The engine call itself is already upstream. |
| `internal/httpapi` — HTTP surface | `net/http` (Go 1.22 routing), graceful shutdown, error mapping | **`ossf/scorecard`** (`scorecard serve`) | Add the `/projects/…` contract to `serve` alongside its existing `GET /?repo=…`. |
| `internal/orchestrator` | Read-through cache: freshness/TTL, single-flight, sync-vs-async (`202`+`Retry-After`) | **`scorecard serve`** (as an optional blob cache) — *the endgame* | This is the genuinely new piece. It has no upstream home today; the target is an opt-in cache layer in `serve`. |
| `internal/tokens`  | SCM token pool + per-host rate limiter + backoff/retry | **`scorecard serve`** (or stays incubator-local) | Needed once `serve` scans concurrently; a single token is unsafe across parallel scans. |
| `internal/httpapi` — `/capabilities` | Server-advertised mode/coverage/freshness/caveats (**D7**) | **New endpoint** upstream + a reader in `scorecard-mcp` | Fixes the MCP hardcoding its provenance to the public cache. Follow-up lives in the `scorecard-mcp` repo, not here. |
| `internal/model`   | Lean in-repo JSON2 mirror (**D13**) | **Does not graft** | Upstream already has `AsJSON2()` and the webapp's generated models. This mirror is a deliberate incubator-local convenience for introspection (`score`→badge; `repo.commit`+`date`→freshness) and passthrough; it stays here. |
| `internal/config`  | 12-factor env config (**D10**) | **Does not graft** | Each upstream project has its own config surface; ours is incubator-local. |

Rule of thumb when editing: keep `store` and the `/projects` handlers **thin and
faithful to the webapp** so they lift out cleanly; keep `scan` a **thin adapter
over `pkg/scorecard`** so v5 API drift is absorbed in one place; treat
`orchestrator`, `tokens`, and `/capabilities` as the incubator's actual novel
contribution.

## `scorecard serve` reconciliation status

`scorecard serve` is the closest existing surface to this server's live path, so
D11 asked whether to **reuse/extend** its handler wiring or **implement fresh and
back-port**. Resolving that required pinning down PR
[#4665](https://github.com/ossf/scorecard/pull/4665)'s true state.

**Resolved (2026-08-05, task 0.1).** PR #4665 **merged 2025-09-10**. It refactored
`scorecard serve` into a REST/HTTP interface, but the author **reverted `chi` back
to the stdlib `net/http`** (Go 1.22 `ServeMux` method+pattern routing). So `main`'s
`serve` today is:

- `net/http` (no `chi`);
- `GET`/`POST /` with `?repo=` (plus package-manager params); `/health`;
- per-request options (race-safe); `scorecard.Run` + `AsJSON2()`;
- an aggregate-score fix and `show_annotations`;
- but **still no store, no cloud dependency**, and it **does not speak the
  `/projects/{host}/{org}/{repo}` contract**.

**Decision.** Implement **fresh** `/projects` handlers in this repo on stdlib
**`net/http` with Go 1.22 routing** — matching `serve`'s own choice (and
`config.yaml`'s "net/http; chi optional" — chi is explicitly not needed for
parity). Reuse `serve`'s proven patterns (per-request options to avoid data
races; `AsJSON2()` for the body). This avoids taking a `chi` dependency purely to
diverge from where upstream landed.

**Endgame.** Graft the `/projects` contract **and** an optional blob cache back
into `scorecard serve`, so the ecosystem converges on **one** HTTP server. The
`orchestrator` seam (a `Store` + `Scanner` behind one read-through cache) is
designed to be that cache layer.

## What deliberately does not graft (v0 non-goals)

These stay out of the upstream graft entirely — they are either deferred
scaling work or internal deployment glue (see the proposal's Non-Goals):

- message broker / distributed workers (`gocloud.dev/pubsub`);
- warm-cache scheduler; analytics/index layer;
- signed-upload `POST`; request-level auth / multi-tenancy;
- BigQuery (read path needs none; it lives only in `scorecard/cron`);
- object-store endpoints, org/repo scan lists, orchestration manifests, and token
  sourcing — internal deployment glue that lives in a separate repo.

## References

- Design **D11** (graft map + serve reconciliation), **D4** (key/body contract),
  **D7** (`/capabilities`), **D13** (result model) —
  `openspec/changes/archive/2026-08-06-add-scorecard-api-server/design.md`.
- Task 0.1 (PR #4665 status) —
  `openspec/changes/archive/2026-08-06-add-scorecard-api-server/tasks.md`.
- `scorecard serve`: `github.com/ossf/scorecard` (`cmd`/`serve`).
- Contract + blob reader: `github.com/ossf/scorecard-webapp`
  (`app/server/get_results.go`).
- Reference client / acceptance test: `github.com/uwu-tools/scorecard-mcp`.
