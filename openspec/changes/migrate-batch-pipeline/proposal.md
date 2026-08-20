# Proposal: Migrate the batch scanning pipeline out of ossf/scorecard

## Why

`ossf/scorecard` currently contains two projects. One is the scan engine: checks,
probes, clients, scoring, output formats — the thing people mean when they say
"Scorecard". The other is `cron/`: a PubSub controller, batch workers, a CII
worker, a BigQuery transfer job, a release webhook, the `projects.csv` inventory,
and the Kubernetes and Cloud Build manifests that run a weekly scan of 1M+
repositories. The engine does not use any of it.

That second project belongs here. `ossf/scorecard-infra` already exists for
exactly this concern, already has module path `github.com/ossf/scorecard-infra`,
already declares `require github.com/ossf/scorecard/v5 v5.5.0`, and is already
doing deliberate work on provider-agnostic Scorecard data infrastructure. `cron/`
arriving later means merging against more infra-side structure, not less.

Three things make now the moment rather than later:

1. **The dependency graph is clean today.** No `.go` file outside `cron/` imports
   `github.com/ossf/scorecard/v5/cron/...`. That single fact is what makes this a
   mechanical move rather than a refactor — and it is not guaranteed to stay
   true. Every month this waits is another month someone can add an inbound edge.
2. **`cron/` is a disproportionate share of upstream's CI surface.** Six Docker
   images, six `ko` targets, two dedicated CI jobs, and five Dependabot paths
   exist solely to serve code the scan engine never calls. Today every
   infrastructure change pays the engine's CI cost, and every engine release
   carries the infrastructure's deployment risk.
3. **The operational history is the runbook.** `cron/` carries 455 commits back
   to 2020-11-10. Quota workarounds, shard sizing, PubSub ack deadlines, and
   BigQuery schema migrations exist *only* as commit messages. A squashed import
   destroys the ability to `git blame` a magic number and learn why it is that
   number. For infrastructure that runs unattended against 1M+ repositories, that
   is not sentimentality — it is the difference between maintainable and
   inherited.

This is a **repository split, not a rewrite.** No cron behavior changes. No
Scorecard check, probe, score, or output format changes.

**Analysis basis:** `ossf/scorecard` `main` @ `d1fab88f`, `ossf/scorecard-infra`
`main` @ `3b4bc7cd`. 128 files under `cron/`; 455 of upstream's 3,104 commits
touch it; 3 files under `clients/githubrepo/roundtripper/tokens/server/`.

## What Changes

- **Import `cron/` in its entirety with full git history**, using
  `git filter-repo` to extract and `git merge --allow-unrelated-histories` to
  graft (**C1**). The merge is conflict-free by construction — this repo has no
  `cron/` path.
- **Import the GitHub token-pool RPC server**
  (`clients/githubrepo/roundtripper/tokens/server/`, 3 files) as
  `cron/internal/githubserver/`. Only the `server/` subdirectory moves; its
  parent `tokens/` package stays upstream, imported by `roundtripper.go` and
  `transport.go` (**C6**).
- **Land `cron/` at the repository top level**, not under `internal/`. Any
  reorganization into this repo's `internal/` + `cmd/` layout happens *after* the
  graft as an ordinary rename commit, so it stays reviewable and revertible
  independently of the history move (**C5**).
- **Protect the imported history**: strip upstream's `v1.x`–`v5.x` tags before
  merging so they do not pollute this repo's release namespace (**C2**), and
  rewrite `(#N)` PR references in the 455 commit subjects to `(ossf/scorecard#N)`
  so GitHub does not auto-link them to unrelated issues here (**C3**).
- **Rewrite import paths at the tip only** — one commit taking
  `github.com/ossf/scorecard/v5/cron/...` to
  `github.com/ossf/scorecard-infra/cron/...`. Rewriting across all 455 commits
  would falsify the record; `git blame` follows content either way (**C4**).
- **Port the build and CI surface**: Makefile build/docker/ko/proto targets, the
  `docker_matrix` image job, the `add-projects` / `validate-projects` jobs, and
  the five cron Dockerfile paths in `dependabot.yml`.
- **Pin the Scorecard engine to a release and add a `main` canary.** `go.mod`
  pins a release, bumped automatically, so production builds are reproducible; a
  separate scheduled job builds and tests the pipeline against `ossf/scorecard`'s
  `main` to recover the PR-time breakage signal the split otherwise gives up. The
  canary carries the schema-drift check too (**C7**/**C8**).
- **Fix the already-stale protobuf `go_package`** in `cron/data/request.proto`
  and `cron/data/metadata.proto` — both declare
  `github.com/ossf/scorecard/cron/data`, missing even the current `/v5` suffix.
- **Cut over to production in stages**: build all six images here to a staging
  tag first, diff against their upstream-built equivalents, then repoint the
  Cloud Build triggers in the `openssf` GCP project. Nothing is deleted upstream
  until production has run clean on infra-built images for a full scan cycle, so
  rollback stays a configuration change with no code restoration (**C10**).
- **Remove the moved code and its wiring from `ossf/scorecard`** last, leaving a
  redirect stub at `cron/README.md` rather than a bare 404 for the bookmarked
  `projects.csv` contribution path (**C9**).

## Capabilities

### New Capabilities

- `batch-pipeline`: the batch scanning components now hosted here — PubSub
  controller, batch worker, CII worker, BigQuery transfer, release webhook, and
  GitHub token-pool server — their preserved runtime behavior, their dependency
  on the published Scorecard engine module, and their ownership of the published
  BigQuery/JSON schema contract.
- `pipeline-deployment`: the deployment surface — six container images, the
  Kubernetes manifests and Cloud Build configs that consume them, the build and
  CI targets that produce them, and the staged cutover and rollback contract.
- `project-inventory`: the `projects.csv` / `gitlab-projects.csv` scan inventory
  as a community contribution surface, its `add-projects` / `validate-projects`
  automation, and the redirect obligation for the relocated contribution path.
- `repository-history`: the verifiable history-preservation guarantees of the
  import — rename-tracked `--follow`, original-author blame, disambiguated PR
  references, and no upstream tag leakage.

### Modified Capabilities

<!--
None. The existing api-server, result-store, result-cache, live-scan,
feature-flags, and upstream-fallback capabilities are untouched: the batch
pipeline lands beside them, not through them. Converging the two — a GCP-bound
batch producer and a provider-agnostic read-through cache that both write
Scorecard results to object storage — is the strategically interesting follow-on
and is deliberately deferred (design C11).
-->

## Impact

- **New code:** `cron/` (128 files) plus `cron/internal/githubserver/` (3 files),
  landing at the repository top level. No existing package under `internal/` or
  `cmd/` is modified.
- **Repository history:** ~455 additional commits dating to 2020-11-10, grafted
  via an unrelated-histories merge. This changes the shape of `git log` for the
  whole repo, permanently and intentionally.
- **Dependencies:** `cron/` consumes 13 packages from `github.com/ossf/scorecard/v5`
  (`pkg/scorecard`, `checker`, `clients`, `clients/githubrepo`,
  `clients/gitlabrepo`, `clients/ossfuzz`, `clients/githubrepo/stats`,
  `docs/checks`, `log`, `errors`, `finding`, `stats`, `policy`). All are already
  exported and reachable through the published module — no new upstream API
  surface is needed. It also adds GCP client libraries (PubSub, BigQuery, GCS) to
  this module's dependency set.
- **Build and CI:** six new image builds, `ko` targets, protobuf generation, and
  two inventory-validation jobs land in this repo's `presubmits.yml` (or a new
  `docker.yml`). Expect a materially longer CI run. One additional **scheduled,
  non-blocking** job builds the pipeline against `ossf/scorecard`'s `main`
  (**C7**/**C8**).
- **External systems:** Cloud Build triggers in the `openssf` GCP project are
  configured outside git and must be repointed at cutover. The change author holds
  admin there, so this is a scheduling constraint rather than a dependency on a
  third party — but it remains the one step that is not reviewable as a diff and
  not revertible by `git revert`.
- **Upstream (`ossf/scorecard`):** loses `cron/`, the token server subdirectory,
  ~45 lines of Makefile, six `docker_matrix` entries, two CI jobs, five
  Dependabot paths, and a `.codecov.yml` ignore. Gains a redirect stub and
  updated `CONTRIBUTING.md` / `AGENTS.md` / `cloudbuild/README.md`.
- **Compatibility:** `cron/data` and `cron/config` are public, non-`internal`
  packages upstream today. Any external importer breaks. None are known and none
  are presumed; Phase 0 verifies rather than discovers. If the check turns
  something up, upstream removal needs a deprecation window instead of a clean
  delete.
- **Governance:** requires Scorecard Steering Committee sign-off, `openssf` GCP
  project admin sign-off on the trigger repointing specifically, and review from
  at least one non-Steering maintainer. The community meeting is notified before
  cutover and again before upstream removal — the `projects.csv` contribution
  path changes for everyone.

## Non-goals

- **Any change to cron runtime behavior**, scan cadence, sharding, or output
  schema. Behavioral equivalence is the acceptance test, not a nice-to-have.
- **Any change to Scorecard checks, probes, scores, or output formats.**
- **Restructuring `cron/` into this repo's `internal/` + `cmd/` layout.**
  Deliberately deferred so the history move and the structural opinion stay
  separately reviewable (**C5**).
- **Consolidating `cron/internal/format`'s `json.v2.schema` with the duplicate
  copy in `pkg/scorecard`.** Tracked separately; not a blocker (**C8**).
- **Migrating cron off GCP.** That is this repo's existing provider-agnostic
  effort. This move feeds it — it is the precondition, not the work.
- **Converging the batch pipeline with the API server's store, cache, or scan
  paths.** Deferred (**C11**).
- **The message broker / distributed workers, the warm-cache scheduler, the
  analytics/index layer, and the signed-upload `POST` path** remain deferred to
  later changes.
