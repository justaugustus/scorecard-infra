# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository. Read this
alongside [`README.md`](README.md), which explains what the repository holds and
how the parts relate.

## What this project is

**OpenSSF Scorecard's hosted infrastructure**, and the work to make it
provider-agnostic. `ossf/scorecard` is the engine; this is everything around it.
Three parts:

1. The **batch scanning pipeline** (`cron/`), imported with history from
   `ossf/scorecard`: the PubSub controller, batch and CII workers, BigQuery
   transfer, release webhook, GitHub token-pool server, and the `projects.csv`
   scan inventories behind the weekly public scan of 1M+ repositories. The
   **producer** — production, GCP-bound, mid-migration.
2. A cloud-agnostic, self-hosted Scorecard results **API server**: a
   read-through cache over an in-process scan engine ("hybrid"). It serves
   pre-computed results from any object store and generates them live on demand.
   It speaks the `ossf/scorecard-webapp` GET contract so it is a drop-in
   `--base-url` target for `uwu-tools/scorecard-mcp`. The **serving tier** — v0,
   provider-agnostic, incubating to graft upstream.
3. The **provider-agnostic design** (`docs/research/`): a reference design for
   running Scorecard's hosted data services on any cloud or self-hosted target.
   Proposal-flavored; nothing in it is committed.

The first two share a repository and nothing else — no import edges in either direction,
enforced by task 3.5 and design **C11**. That is intentional for now. The
pipeline is a GCP-bound producer writing Scorecard results to a bucket; the API
server is a provider-agnostic consumer reading them from one. Converging them —
putting `gocloud.dev/blob` under the pipeline and letting the orchestrator serve
batch-produced results — is what landing them together makes possible, and is
tracked separately.

**Do not "tidy" this by wiring the two together.** Any convergence needs its own
spec; a behavior-preserving migration is what keeps the split revertible.

## Status

| Change | State |
| --- | --- |
| `archive/2026-08-06-add-scorecard-api-server` | **Done** (45/46), archived. Canonical specs now live under `openspec/specs/`; the original proposal, design, and tasks are preserved under the archive path. |
| `add-upstream-fallback` | 26/27 — implemented; see `internal/fallback`. |
| `add-feature-flagging` | 14/15 — implemented; see `internal/flags`. |
| `migrate-batch-pipeline` | **39/64 — in flight.** Import and build/CI wiring landed; staging image diff, Cloud Build trigger cutover, a clean scan cycle, and upstream removal remain. |

The API server's v0 core is implemented and tested — model, store, scan/tokens,
orchestrator, HTTP contract, config, and the wired binary — plus the acceptance
gate: a real `scorecard-mcp` binary (stdio, `-base-url`) verified against a live
server for both a cache HIT and a MISS→live-scan→persist→HIT on `fileblob` and,
via the local Docker Compose dev environment, on a self-hosted S3-compatible
store too (see `docs/acceptance.md`).

The pipeline is **not** finished migrating. Until cutover completes it stays
behavior-frozen — see [The batch pipeline (cron/)](#the-batch-pipeline-cron).

## Where to start

1. Read [`README.md`](README.md) for what the repository is and how the parts
   relate.
2. Read the canonical specs: `openspec list --specs`, then read
   `openspec/specs/{api-server,result-store,result-cache,live-scan}/spec.md`.
3. Read [`openspec/config.yaml`](openspec/config.yaml) for durable project context.
4. Read the active changes under `openspec/changes/` before starting anything —
   `migrate-batch-pipeline` in particular constrains what may be touched.
5. For new work, propose a new OpenSpec change under `openspec/changes/`, keeping
   the specs in `openspec/specs/` and the code in sync.

[`docs/bootstrap.md`](docs/bootstrap.md) is a **historical record** of the v0
design brief, not onboarding — it predates the repository's widening and some of
its instructions are superseded. Read it for *why* v0 is shaped the way it is.

## Architecture (internal/)

- `model/` — lean JSON2 + provenance types (design **D13**; not the webapp's
  generated models).
- `store/` — `gocloud.dev/blob`; backend chosen by a URL env var; the
  `{host}/{org}/{repo}[/{commit}]/results.json` key contract (**D3/D4**).
- `orchestrator/` — the read-through cache seam: freshness/TTL, single-flight,
  sync-vs-async (**D2/D5/D6**).
- `scan/` — wraps `pkg/scorecard.Run`; reused clients; JSON2; write-back (**D8**).
- `tokens/` — SCM token pool + per-host rate limiter/backoff (**D8**).
- `flags/` — runtime feature-flag seam over OpenFeature; in-process, env-seeded
  static provider by default (**FF1/FF2/FF5**).
- `cmd/scorecard-api/` — the binary.

## The batch pipeline (cron/)

Imported from `ossf/scorecard` with full history. Read
[`cron/initial-graft.md`](cron/initial-graft.md) before working in this tree — it
records what came across, what was hand-ported, and where to trace the rest.

- `cron/internal/controller/` — PubSub batch controller
- `cron/internal/worker/` — batch scan worker
- `cron/internal/cii/` — CII best-practices worker
- `cron/internal/bq/` — BigQuery transfer
- `cron/internal/webhook/` — release webhook
- `cron/internal/githubserver/` — GitHub token-pool RPC server (relocated from
  `clients/githubrepo/roundtripper/tokens/server/`; its parent `tokens/` package
  stays in `ossf/scorecard`)
- `cron/internal/format/` — **owns the published BigQuery/JSON schema contract**
- `cron/internal/data/` — `projects.csv` / `gitlab-projects.csv` scan inventories
- `cron/k8s/`, `cron/cloudbuild/` — deployment manifests and image build configs

Two things to know before changing anything here. The tree is **behavior-frozen**
until production cutover completes: it must stay equivalent to what
`ossf/scorecard` builds, because that equivalence is the migration's acceptance
test and its rollback path. And `cron/internal/format` serializes a data model
that lives upstream, so a schema edit here without the corresponding engine change
breaks the contract silently — `schema_gen_test.go` is what catches it.

## Build, test, lint

```sh
make build      # go build ./...
make test       # go test ./... -race
make lint       # golangci-lint run ./...
```

Everything must be clean before a change is considered done.

Pipeline-specific targets (`make help` lists them all):

```sh
make build-controller build-worker ...   # the seven pipeline binaries
make dockerbuild                         # all six container images
make ko-images                           # the same six via ko
make validate-projects                   # validate the scan inventories
make add-projects                        # normalize inventory additions
make build-proto                         # regenerate protobufs (requires protoc)
```

`ko` and `protoc` are expected on `PATH` rather than vendored; the Makefile says
why, and errors actionably when they are missing. `build-proto` is deliberately
explicit rather than a file rule — run it when you change a `.proto`, and commit
the generated output.

**Toolchain note:** match the Scorecard Go toolchain version (see `go.mod`). If
builds fail with a Go tool version mismatch across many stdlib packages, a stray
`GOROOT` is the cause — prefix commands with `env -u GOROOT`.

Configuration is env-driven; `internal/config` documents every variable and its
default. Workflows are also linted with `actionlint` and `zizmor`.

### Linting conventions (aligned with ossf/scorecard's strict config)

The `.golangci.yml` is a near-verbatim port of Scorecard's. A few `//nolint`
patterns are intentional and should be preserved when editing:

- `wrapcheck` ignores this module's own packages: their errors are already
  contextualized at the source (`store: …`, `tokens: …`), so re-wrapping across
  our own boundaries would only double the prefix. Third-party errors must still
  be wrapped with `%w`.
- `//nolint:govet` (fieldalignment) on the JSON2 mirror types (`model.Result`,
  `model.Check`) keeps canonical wire field order, and on lifetime singletons
  (`scan.EngineScanner`) where field order documents client reuse. Elsewhere,
  order fields for pointer packing instead of suppressing.
- `//nolint:contextcheck` where a background scan or graceful shutdown
  intentionally uses a fresh context that must outlive the request.
- Define package-level sentinel errors (`err113`) rather than returning dynamic
  `errors.New`/`fmt.Errorf` values inline.

## Cloud-agnostic rules (non-negotiable)

- No hardcoded bucket URLs (no `gs://…` constants). All config via environment:
  bucket URL, TTL, timeouts, enabled checks, worker concurrency, listen port, SCM
  credentials.
- Blank-import every blob driver (`s3blob`, `azureblob`, `gcsblob`, `fileblob`,
  `memblob`); credentials resolve via each backend's default chain.
- **No BigQuery.** Local dev = `fileblob`; local S3 = a self-hosted
  S3-compatible store; tests = `memblob`.

## Feature flags vs. configuration

Two distinct mechanisms; classify every new setting deliberately (design **FF3**):

- **Configuration** — endpoints, credentials, timeouts, TTLs, tuning. Env-driven,
  validated at startup, fail-fast. Lives in `internal/config`.
- **Feature flags** — runtime *behavioral* toggles and rollouts (on/off, mode
  selection). Read through `internal/flags` with a safe in-code default, and a
  capability owns and declares its own flags. Backed by an in-process, env-seeded
  static provider today (per-flag overrides via `SCORECARD_FLAG_*`);
  `SCORECARD_FLAGS_PROVIDER` selects the provider.

The test: if you would change it *without a redeploy* (incident kill-switch,
gradual rollout, mode shift), it is a flag; if the process needs it to boot or
connect, it is configuration.

## Responsible framing

Scorecard results are heuristic **signals, not a verdict**. Never assert a repo
"is secure/insecure." Every response declares its `source` (cached vs. live),
freshness, and completeness. A score of `-1` is inconclusive, not failing.

## Commit and PR conventions

- **DCO sign-off** on every commit: `git commit -s`. This is the *human's*
  certification — an AI agent must **never** add its own `Signed-off-by:`.
- **AI attribution** for assisted work, following the Linux kernel
  [coding-assistants convention](https://docs.kernel.org/process/coding-assistants.html):
  an `Assisted-by: AGENT:MODEL` trailer (e.g. `Assisted-by: Claude Code:claude-opus-5`).
  Not `Co-Authored-By:` — earlier docs in this repo specified that and were wrong.
- Single **atomic commit** per logical change, with a detailed body explaining the
  *why* (reference design decisions, e.g. **D5**/**C11**, and OpenSpec tasks).
- Work on **feature branches**; never commit directly to `main`. Do not open PRs
  unless explicitly asked.
- **No employer/internal references** in files or commits. Internal deployment
  glue lives in a separate repo.

## Upstream graft map

This is an incubator, not a permanent fork. Structure code so durable pieces graft
upstream cleanly (design **D11**): the contract + blob read path → `ossf/scorecard-webapp`;
the live scan + HTTP surface → `ossf/scorecard`'s `scorecard serve`. The full
per-component graft map and the `scorecard serve` reconciliation status live in
[`docs/upstream-graft.md`](docs/upstream-graft.md).
