# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository. Read this
alongside [`docs/bootstrap.md`](docs/bootstrap.md), which onboards a fresh session
with the full project context.

## What this project is

A cloud-agnostic, self-hosted OpenSSF Scorecard results **API server**: a
read-through cache over an in-process scan engine ("hybrid"). It serves
pre-computed results from any object store and generates them live on demand. It
speaks the `ossf/scorecard-webapp` GET contract so it is a drop-in `--base-url`
target for `uwu-tools/scorecard-mcp`.

## Where to start

1. Read [`docs/bootstrap.md`](docs/bootstrap.md).
2. Read the OpenSpec change: `openspec list`, then
   `openspec show 2026-08-05-add-scorecard-api-server` (or read the files under
   `openspec/changes/2026-08-05-add-scorecard-api-server/`:
   `proposal.md`, `design.md`, `tasks.md`, and `specs/`).
3. Read [`openspec/config.yaml`](openspec/config.yaml) for durable project context.
4. Implement against `tasks.md`, keeping specs and code in sync.

## Architecture (internal/)

- `model/` — lean JSON2 + provenance types (design **D13**; not the webapp's
  generated models).
- `store/` — `gocloud.dev/blob`; backend chosen by a URL env var; the
  `{host}/{org}/{repo}[/{commit}]/results.json` key contract (**D3/D4**).
- `orchestrator/` — the read-through cache seam: freshness/TTL, single-flight,
  sync-vs-async (**D2/D5/D6**).
- `scan/` — wraps `pkg/scorecard.Run`; reused clients; JSON2; write-back (**D8**).
- `tokens/` — SCM token pool + per-host rate limiter/backoff (**D8**).
- `cmd/scorecard-api/` — the binary.

## Build, test, lint

```sh
go build ./...
go test ./... -race
golangci-lint run ./...        # config in .golangci.yml (aligned with ossf/scorecard)
```

Everything must be clean before a change is considered done.

**Toolchain note:** match the Scorecard Go toolchain version (see `go.mod`). If
builds fail with a Go tool version mismatch across many stdlib packages, a stray
`GOROOT` is the cause — prefix commands with `env -u GOROOT`.

## Cloud-agnostic rules (non-negotiable)

- No hardcoded bucket URLs (no `gs://…` constants). All config via environment:
  bucket URL, TTL, timeouts, enabled checks, worker concurrency, listen port, SCM
  credentials.
- Blank-import every blob driver (`s3blob`, `azureblob`, `gcsblob`, `fileblob`,
  `memblob`); credentials resolve via each backend's default chain.
- **No BigQuery.** Local dev = `fileblob`; local S3 = MinIO; tests = `memblob`.

## Responsible framing

Scorecard results are heuristic **signals, not a verdict**. Never assert a repo
"is secure/insecure." Every response declares its `source` (cached vs. live),
freshness, and completeness. A score of `-1` is inconclusive, not failing.

## Commit and PR conventions

- **DCO sign-off** on every commit: `git commit -s`.
- Single **atomic commit** per logical change, with a detailed body explaining the
  *why* (reference design decisions, e.g. **D5**, and OpenSpec tasks).
- Work on **feature branches**; never commit directly to `main`. Do not open PRs
  unless explicitly asked.
- **No employer/internal references** in files or commits. Internal deployment
  glue lives in a separate repo.

## Upstream graft map

This is an incubator, not a permanent fork. Structure code so durable pieces graft
upstream cleanly (design **D11**): the contract + blob read path → `ossf/scorecard-webapp`;
the live scan + HTTP surface → `ossf/scorecard`'s `scorecard serve`.
