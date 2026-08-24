# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository. **Read
[`README.md`](README.md) first** — it explains what the repository holds, how
the parts relate, and why. This file holds only what README and the code don't
already cover: active constraints, gotchas, and conventions.

This repository has three parts, sharing a repo and nothing else — no import
edges either direction: the **batch scanning pipeline** (`cron/`, production,
GCP-bound, mid-migration), the **results API server** (`internal/`, v0,
provider-agnostic, incubating to graft upstream), and the **provider-agnostic
design** (`docs/research/`, proposal-flavored).

**Do not "tidy" this by wiring the pipeline and the API server together.** Any
convergence needs its own spec (design **C11**); a behavior-preserving migration
is what keeps the current split revertible.

## Current state

Don't trust a remembered fraction — task counts change. Get the live state from:

- `openspec list --specs` — canonical specs (implemented, done)
- `openspec/changes/` — active proposals; `tasks.md` in each has the real
  checklist. `migrate-batch-pipeline` is the one still in flight and constrains
  what may be touched in `cron/`.
- `openspec/changes/archive/` — completed and archived proposals

## Where to start

1. Read [`README.md`](README.md).
2. Read the canonical specs: `openspec list --specs`, then
   `openspec/specs/{api-server,result-store,result-cache,live-scan}/spec.md`.
3. Read [`openspec/config.yaml`](openspec/config.yaml) for durable project context.
4. Read the active changes under `openspec/changes/` before starting anything.
5. For new work, propose a new OpenSpec change under `openspec/changes/`, keeping
   the specs in `openspec/specs/` and the code in sync.

[`docs/bootstrap.md`](docs/bootstrap.md) is a **historical record** of the v0
design brief, not onboarding — it predates the repository's widening and some of
its instructions are superseded. Read it for *why* v0 is shaped the way it is.

## Design-decision map (internal/)

README documents what each package does; these tags point back to *why* it's
shaped that way. The tags below appear only here — not in code comments — so
this is the map:

- `model/` — D13 (lean JSON2 + provenance, not the webapp's generated models)
- `store/` — D3/D4 (backend-by-URL, key contract)
- `orchestrator/` — D2/D5/D6 (freshness/TTL, single-flight, sync-vs-async)
- `scan/`, `tokens/` — D8 (client reuse, token pool)
- `flags/` — FF1/FF2/FF5 (runtime feature-flag seam)
- Upstream graft plan — D11; see [`docs/upstream-graft.md`](docs/upstream-graft.md)
  for the per-component map and reconciliation status.

## The batch pipeline (cron/)

Imported from `ossf/scorecard` with full history. Read
[`cron/initial-graft.md`](cron/initial-graft.md) before working in this tree.

Two rules apply here and nowhere else. The tree is **behavior-frozen** until
production cutover completes — it must stay equivalent to what `ossf/scorecard`
builds, because that equivalence is the migration's acceptance test and its
rollback path. And `cron/internal/format` serializes a data model that lives
upstream, so a schema edit here without the corresponding engine change breaks
the contract silently — `schema_gen_test.go` is what catches it.

## Build, test, lint

```sh
make build      # go build ./...
make test       # go test ./... -race
make lint       # golangci-lint run ./...
make help       # every target, including the pipeline-specific ones
```

Everything must be clean before a change is considered done. Workflows are also
linted with `actionlint` and `zizmor`.

**Toolchain note:** match the Scorecard Go toolchain version (see `go.mod`). If
builds fail with a Go tool version mismatch across many stdlib packages, a stray
`GOROOT` is the cause — prefix commands with `env -u GOROOT`.

Define package-level sentinel errors (`err113`) rather than returning dynamic
`errors.New`/`fmt.Errorf` values inline — the other `.golangci.yml`
repo-specific `//nolint` exceptions are explained in comments at their call
sites (`wrapcheck` in `.golangci.yml`; `fieldalignment`/`contextcheck` at each
use).

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
"is secure/insecure." A score of `-1` is inconclusive, not failing. (See
README's framing note for the full statement — this applies to any code path
that formats or reports a score, not just user-facing docs.)

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
