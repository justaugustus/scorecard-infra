# Historical record: the v0 API server brief

> [!IMPORTANT]
> **This is a point-in-time record, not onboarding.** It is the brief that
> produced the `add-scorecard-api-server` change — written *before* that work
> existed, when this repository held nothing but the API server. It is preserved
> because it explains *why* the v0 design is shaped the way it is, and because
> several of its decisions are still load-bearing.
>
> Do not follow its instructions. The work it briefs is **done and archived**, the
> repository has since widened to hold the batch scanning pipeline as well, and
> some of its conventions have been superseded.
>
> | For | Read |
> | --- | --- |
> | What this repository is now | [`README.md`](../README.md) |
> | Current conventions and tooling | [`AGENTS.md`](../AGENTS.md), [`CONTRIBUTING.md`](../CONTRIBUTING.md) |
> | Current behavior (canonical) | `openspec/specs/{api-server,result-store,result-cache,live-scan}/spec.md` |
> | The change this brief produced | `openspec/changes/archive/2026-08-06-add-scorecard-api-server/` |
> | Durable project context | [`openspec/config.yaml`](../openspec/config.yaml) |

The original text follows, unedited except where a stale instruction would
actively mislead.

## What this briefed

A cloud-agnostic, self-hosted (non-OpenSSF) OpenSSF Scorecard results API server.
It is a **read-through cache over an in-process scan engine** ("hybrid"): it serves
pre-computed results and generates them live on demand. It is a drop-in `--base-url`
target for [`uwu-tools/scorecard-mcp`](https://github.com/uwu-tools/scorecard-mcp).

That description is still accurate for `cmd/scorecard-api` — but it is no longer a
description of the repository, which also hosts the batch scanning pipeline
(`cron/`). See the README.

## Contract (match ossf/scorecard-webapp exactly)

- `GET /projects/{host}/{org}/{repo}[?commit=SHA]` -> `ScorecardResult` (JSON2)
- `GET /projects/{host}/{org}/{repo}/badge` -> SVG
- `GET /capabilities` -> this server's mode/coverage/freshness/caveats (NEW)
- `GET /health`, `GET /readyz`

Blob object keys (must match the webapp exactly):

```
{host}/{org}/{repo}/results.json            # latest
{host}/{org}/{repo}/{commit}/results.json   # pinned, immutable
```

Body is canonical Scorecard **JSON2**.

## Chosen layout

```
cmd/scorecard-api/
internal/
  model/         JSON2 + provenance types
  store/         gocloud.dev/blob; s3/azure/gcs/file/mem by URL env; key contract   [port from scorecard-webapp]
  orchestrator/  read-through cache: freshness/TTL, single-flight, sync/async        [NEW — the brain]
  scan/          wraps pkg/scorecard.Run; reused clients; JSON2; writes back to store [ossf/scorecard engine + oss-pulse pattern]
  tokens/        SCM token pool + per-host rate limiter/backoff/retry                 [NEW — critical]
```

## Cloud-agnostic rules (non-negotiable)

- No hardcoded `gs://` anything. All config via env: bucket URL, `latest` TTL,
  request/scan timeouts, enabled checks, worker concurrency, listen port, SCM creds.
- Blank-import all blob drivers: `s3blob`, `azureblob`, `gcsblob`, `fileblob`,
  `memblob`. Creds resolve via each backend's default chain.
- **No BigQuery.** Local dev = `fileblob`; local S3 = a self-hosted
  S3-compatible store; tests = `memblob`.

## Pre-work resolved before the HTTP layer (tasks 0.x)

These were open questions at the time of writing. **0.1 has since been resolved** —
PR #4665 merged 2025-09-10 and reverted `chi` back to stdlib `net/http`; the
decision and its consequences are recorded in
[`upstream-graft.md`](upstream-graft.md#scorecard-serve-reconciliation-status).

- **0.1 — Verify PR #4665's true merge status** in `ossf/scorecard` and decide whether
  to reuse/extend `scorecard serve`'s handler wiring or implement fresh and back-port.
  `scorecard serve` is the closest existing surface (on-demand, `net/http`,
  `GET /?repo=…`, no store, no cloud deps) but does **not** speak the `/projects`
  contract. `main` still uses `net/http`, so treat #4665's chi/REST refactor as
  unlanded until confirmed. See design **D11**.
- **0.2 —** Decide: import `scorecard-webapp` generated models directly vs. vendor
  `openapi.yaml` and regenerate.
- **0.3 —** Confirm the blob key + JSON2 body contract against a real
  `scorecard-webapp` object (design **D4**).

## Why `/capabilities` exists

`scorecard-mcp` currently hardcodes the public cache's caveats
(`internal/provider/rest.go`: opted-in-only; weekly scan omits three checks). Those
are **wrong** for this server (it scans on demand, all checks, any accessible repo).
The server advertises its own caveats/coverage/freshness at `/capabilities` so clients
report provenance correctly. Follow-up (in the scorecard-mcp repo, not here): teach the
MCP to read `/capabilities` instead of hardcoding.

## Upstream graft map (this is an incubator, not a permanent fork)

- Contract handlers + blob read path -> `ossf/scorecard-webapp`
  (bucket-URL parameterization + driver imports).
- Live scan + HTTP surface -> `ossf/scorecard` `scorecard serve`
  (endgame: teach it the `/projects` contract + an optional blob cache).

Structure code so these split cleanly.

## v0 scope

- **IN:** GET result (latest + commit), badge, `/capabilities`, blob cache, live-scan
  fallback via an in-process worker pool, token/rate manager.
- **OUT (deferred):** message broker (`gocloud.dev/pubsub`), warm-cache scheduler,
  analytics/index (DuckDB/Postgres/ClickHouse), signed-upload `POST` (Sigstore),
  request-level auth/multi-tenancy.

## v0 done =

`scorecard-mcp --base-url http://localhost:PORT get_repo_score <repo>` returns a
correct result: a cache **HIT** from a `fileblob` bucket, and a cache **MISS** that
triggers a live `scorecard.Run()`, populates the bucket, and serves it — with an
integration test proving both paths against `fileblob` and a self-hosted
S3-compatible store.

## Conventions

- Spec-driven via OpenSpec: explore -> propose -> design -> specs -> tasks ->
  implement. Keep specs and code in sync.
- Go: match the Scorecard toolchain version. If builds fail with a go tool version
  mismatch across many stdlib packages, a stray `GOROOT` is the cause — prefix
  commands with `env -u GOROOT`.
- Run `golangci-lint` before considering anything done; `go test ./...` clean.
- Commits: single atomic commit per logical change; DCO sign-off (`git commit -s`);
  detailed body. Feature branches only — never commit to `main`. Do not open PRs
  unless explicitly asked. *(Superseded: this brief specified a
  `Co-Authored-By: Claude` trailer. The repository now uses an `Assisted-by:`
  trailer for AI-assisted work, and an AI never adds its own `Signed-off-by:` —
  only a human can certify the DCO. See [`AGENTS.md`](../AGENTS.md).)*
- OSS repo: no employer/internal references in files or commits. Internal deployment
  glue (object-store endpoints, org/repo scan lists, orchestration manifests, token
  sourcing) lives in a separate internal repo that deploys and feeds this server.

## Boundaries

- Reference client / acceptance test: `uwu-tools/scorecard-mcp`.
- Engine + `scorecard serve`: `github.com/ossf/scorecard`.
- Contract + blob reader + models: `github.com/ossf/scorecard-webapp`.
