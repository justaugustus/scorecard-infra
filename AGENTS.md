# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository. **Read
[`README.md`](README.md) first** — it explains what the repository holds, how
the parts relate, and why. This file holds only what README and the code don't
already cover: active constraints, gotchas, and conventions.

This repository has four parts, sharing a repo and nothing else — no import
edges in any direction:

| Part | What it is |
| --- | --- |
| `cron/` | **Batch scanning pipeline.** Imported from `ossf/scorecard` with history. Production on GCP; **cutover to AWS still ahead**, so keep it equivalent to what upstream builds. |
| `api/` | **The results API** serving `api.scorecard.dev`. Imported from `ossf/scorecard-webapp` with history. **Production on AWS; cutover complete.** This is the server that ships. |
| `internal/`, `cmd/` | **A provider-agnostic hybrid API server**, built here. Implements the same contract as `api/`. Currently **off the deployment path** — see below. |
| `docs/research/` | The provider-agnostic design. Proposal-flavored. |

**Do not "tidy" this by wiring the three code trees together.** Any convergence
needs its own spec (designs **C11** and **W10**); behavior-preserving migrations
are what keep the imports revertible. The missing import edges are deliberate,
not an oversight, and `presubmits.yml`'s `import-edges` job enforces them.

## Which API server ships (read before touching either)

`api/` and `internal/httpapi` both implement the results contract. **`api/` is
what deploys.** It is the code already serving production against every live
consumer, and re-hosting it is a lift-and-shift, not a refactor.

`internal/httpapi` and its supporting packages are not deleted and not
unmaintained — but they are not the forward path today. Do not add new API
surface there on the assumption it will ship. The decision is revisitable; it is
not ambiguous. See [`docs/upstream-graft.md`](docs/upstream-graft.md).

## Quarantined: do not "fix" these

Both imported trees violate rules stated further down this file. That is
deliberate: for `cron/`, reverting the violation breaks the cutover's acceptance
test, which is that the deployed behavior did not change. Do not "fix" these
opportunistically — each needs its own change.

- **`api/openapi.yaml` carries an `x-google-backend` block.** It is both the
  published contract and the GCP API gateway's deployment configuration.
  **Re-evaluate:** the API now serves from AWS, so this block is likely
  vestigial — but confirm the Endpoints gateway is actually decommissioned
  before touching it, and do it as its own change.
- **`cron/` retains its own GCS write path.** Its cutover has not happened, so
  the original reasoning still holds (**C11**).
- **`.golangci.yml` applies a narrower linter set to `api/`.** The imported tree
  ran a different config upstream; the block is scoped to `api/` and marked for
  removal, not a general relaxation. Its condition — the `api/` cutover
  completing — is now met; see `migrate-api` task 5.7.

`api/app/server`'s hardcoded `gs://` bucket URLs were on this list and are not
any more — they became `SCORECARD_RESULTS_BUCKET_URL` and
`SCORECARD_CRON_RESULTS_BUCKET_URL` in `configure-result-buckets`, defaulting to
the values that had been compiled in. That is what lifting an item here looks
like: its own change, with defaults that preserve the deployed behavior. It is
not a precedent for fixing the remaining three opportunistically.

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

Two rules apply here and nowhere else. **Keep the tree equivalent to what
`ossf/scorecard` builds** until production cutover completes, because that
equivalence is the cutover's acceptance test and its rollback path. And
`cron/internal/format` serializes a data model that lives upstream, so a schema
edit here without the corresponding engine change breaks the contract silently —
`schema_gen_test.go` is what catches it.

## The results API (api/)

Imported from `ossf/scorecard-webapp` with full history. Read
[`api/initial-graft.md`](api/initial-graft.md) before working in this tree.

Upstream's layout is intact inside it — `api/app/server/` is hand-written,
`api/app/generated/` is derived from `api/openapi.yaml` by go-swagger — so paths
stay 1:1 with upstream while both copies exist. It keeps its own `Makefile`,
whose recipes are relative to `api/`; the root Makefile delegates via
`api-build`, `api-swagger`, and `api-docker`.

Three things to know before editing:

- **`api/app/generated/restapi/configure_scorecard.go` is hand-owned** despite
  living in the generated tree. It holds the handler wiring, CORS setup, and JSON
  producer config, and is excluded from `SWAGGER_GEN`. A regeneration that
  overwrites it silently reverts routing.
- **The end-to-end specs reach the network.** They call `api.github.com` (CI
  supplies a token; without one they hit the 60/hour unauthenticated limit and
  flake) and live Rekor. They pass in CI. They will *skip* rather than fail if
  Rekor's search-by-hash index is unreachable — a guard upstream added because
  Rekor v2 removed that index — so on a network that blocks Rekor you will see
  skips locally and full coverage in CI. Check the skip count before concluding
  anything from a local run.
- **The binary is still called `scorecard-webapp`** while this repository's own is
  `scorecard-api`, which inverts what each one actually is. Deliberate: renaming
  changes image contents, and image equivalence is the cutover gate. It gets fixed
  after cutover, not before.

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
