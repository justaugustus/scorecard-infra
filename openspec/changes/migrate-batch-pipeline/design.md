# Design: Migrate the batch scanning pipeline out of ossf/scorecard

## Context

`ossf/scorecard`'s `cron/` tree is the deployment and data-pipeline layer behind
the weekly public scan of 1M+ repositories. It is operationally significant,
architecturally separate from the scan engine, and — measured by CI surface and
release risk — an expensive lodger in a repository whose reason to exist is the
engine.

Two properties of the current state determine the whole design:

**The coupling is one-directional and shallow.** Nothing outside `cron/` imports
`cron/`:

```console
$ grep -rlE '"github\.com/ossf/scorecard/v5/cron' --include="*.go" . | grep -v '^cron/'
(no results)
```

Outbound, `cron/` uses 13 packages from the engine, all already exported and
reachable through the published module. So the move requires no new upstream API
surface and no engine refactor. That is unusual and it is the reason this can be
a mechanical split.

**The history is clean to extract.** Every historical rename touching `cron/` has
both endpoints inside `cron/` (`cron/worker/main.go` →
`cron/internal/worker/main.go`; `cron/internal/data/writer.go` →
`cron/data/writer.go`). Nothing was ever moved *into* `cron/` from elsewhere in
the tree, so a path-based filter cannot truncate any file's lineage.

The hard parts are therefore not the Go code. They are: the non-Go build wiring
(Makefile, workflows, Dependabot, codecov), the Cloud Build triggers that live in
a GCP project rather than in git, and the community contribution path through
`projects.csv` that thousands of people have bookmarked.

## Goals / Non-Goals

**Goals:**

- Land `cron/` here with history, blame, and rename tracking genuinely intact —
  not "history preserved" in the sense of one import commit with a long message.
- Change no cron runtime behavior. Behavioral equivalence between upstream-built
  and infra-built images is a gate, not an aspiration.
- Keep rollback cheap for as long as possible: a configuration change, not a code
  restoration.
- Leave the community's `projects.csv` contribution path working, or at minimum
  self-explaining, after the move.
- Keep each phase independently reviewable and independently revertible.

**Non-Goals** (see proposal): runtime/schema changes, engine changes,
restructuring into `internal/` + `cmd/`, schema consolidation, the GCP exit
itself, and convergence with the API server's store/cache paths.

## Decisions

**Status:** C1–C13 are resolved. Three were settled against the initial
recommendation and are marked below: **C7** gained a `main` canary, **C8** gained
a scheduled cross-repo schema check, and **C13** was added during the extraction
when the shared-root-file constraint surfaced. The only Phase 0 item still
genuinely open is the external-importer check (see *Open questions*).

### C1 — `git filter-repo`, not `git subtree split` or `filter-branch`

`git filter-repo` is the upstream-recommended replacement for the deprecated
`git filter-branch`. It rewrites the commit graph to contain only the selected
paths while preserving authorship, dates, messages, and rename detection across
the retained history.

`git subtree split --prefix=cron` produces a comparable result but is dramatically
slower across a 3,104-commit history and has weaker rename handling. Rejected.
`filter-branch` is deprecated and slower still. Rejected.

The filter runs against a **fresh single-branch clone**, never a working
checkout: `filter-repo` rewrites history irreversibly and detaches the origin
remote by design.

```bash
git clone --single-branch --branch main \
  https://github.com/ossf/scorecard.git scorecard-cron-extract
cd scorecard-cron-extract
git tag -l | xargs -r git tag -d                    # C2
git filter-repo \
  --path cron/ \
  --path clients/githubrepo/roundtripper/tokens/server/ \
  --path-rename clients/githubrepo/roundtripper/tokens/server/:cron/internal/githubserver/ \
  --message-callback '
      return re.sub(rb"\(#([0-9]+)\)", rb"(ossf/scorecard#\1)", message)
  '                                                  # C3
```

`re` is available in the `--message-callback` context because `git-filter-repo`
imports it at module scope. Confirm this on the dry run; if the installed version
differs, add `import re` to the callback body.

Grafting uses `git merge --allow-unrelated-histories`. It is conflict-free by
construction: this repository has no `cron/` path.

### C2 — Strip upstream tags before the graft

A full clone of `ossf/scorecard` carries every `v1.x`–`v5.x` tag. Merging without
deleting them imports upstream's entire tag namespace into this repository,
polluting its release list and breaking its own versioning. This is trivial to
prevent and unpleasant to undo after a push, so it is a gate, not a step.

### C3 — Rewrite PR references during the filter

455 commit subjects end in `(#1234)`. GitHub auto-links those to the *containing*
repository, so after the graft every one would resolve to an unrelated
`ossf/scorecard-infra` issue or PR. Rewriting `(#N)` → `(ossf/scorecard#N)` in
the filter makes them resolve correctly and forever.

This must happen during the filter. Fixing it afterwards means rewriting grafted
history in a repository other people have already pulled.

### C4 — Rewrite import paths at the tip, not across history

Rewriting `github.com/ossf/scorecard/v5/cron/...` →
`github.com/ossf/scorecard-infra/cron/...` in all 455 commits would make each one
self-consistent and independently buildable. It would also be false: those
commits genuinely contained the old paths.

A single tip commit is honest, reviewable as a diff, and costs nothing that
matters — `git blame` follows content moves regardless of path rewriting. The
only capability given up is checking out an arbitrary historical cron commit and
building it, which no one has asked for.

### C5 — Land `cron/` at the top level; reorganize later, separately

This repo uses an `internal/` + `cmd/` layout, so a top-level `cron/` is
inconsistent with it. The alternative — `--path-rename cron/:internal/cron/`
during the filter — reaches a cleaner end state in one step, but it bakes a
structural opinion into the history import and makes `cron/data` and
`cron/config` unimportable by any external consumer as a side effect of a move
that was supposed to be mechanical.

Land it flat. Do any reorganization afterwards as an ordinary rename commit, which
both `git blame` and `--follow` track, and which can be reviewed and reverted on
its own. Separating "move it" from "reorganize it" is worth one interim
inconsistency.

### C6 — Move only the token server subdirectory

`clients/githubrepo/roundtripper/tokens/server/` (`main.go`, `Dockerfile`,
`cloudbuild.yaml`) is the GitHub token-pool RPC server. It is cron-only
infrastructure — deployed by `cron/k8s/auth.yaml`, built by
`cron-github-server-docker` — that lives under `clients/` for historical reasons.

Its parent package `clients/githubrepo/roundtripper/tokens/` (accessor,
round-robin, RPC client) **must stay upstream**: `roundtripper.go` and
`transport.go` import it. The `server/` subdirectory imports nothing but that
public parent, so it moves cleanly on its own.

Destination `cron/internal/githubserver/` matches the existing
`cron-github-server` make target and `scorecard-github-server` image name.

Leaving it upstream was the alternative — it costs upstream one
indefinitely-maintained cron image and leaves the split incomplete. **Decided:
move it**, so upstream builds no cron image at all after removal.

### C7 — Pin the engine to a release, with an automated bump *and* a `main` canary

Today the cron worker builds against in-tree scorecard, so an engine change that
breaks cron fails at PR time. After the split it builds against whatever `go.mod`
pins (`v5.5.0` currently). Two things are worth having and they pull in opposite
directions: reproducible production builds, and early warning when the engine
breaks the pipeline.

The two obvious options each sacrifice one. Pinning to a release with an automated
bump gives reproducibility but moves breakage discovery to bump time — a real
regression in feedback latency, and the bump lands on whoever happens to be
reviewing dependency PRs rather than on the author of the change. Tracking `main`
via a pseudo-version keeps the feedback loop but makes production builds
non-reproducible and lets an upstream `main` breakage reach the weekly scan.

**Decided: take both halves.** `go.mod` pins a release and is bumped
automatically, so production is reproducible and runs a known engine version.
Separately, a **scheduled CI job builds and tests the pipeline against
`ossf/scorecard`'s `main`**. That job does not gate this repository's pull
requests — it is testing someone else's branch, and a red light caused by upstream
work in progress must not block unrelated changes here. It does need to be
visible: a canary nobody looks at is worse than no canary, because it manufactures
confidence. Route its failures somewhere a maintainer actually reads, and treat a
sustained red canary as a signal to talk to the engine maintainers before the
next bump lands rather than after.

This costs one non-blocking scheduled job and recovers most of what the split
otherwise gives up. It does not fully restore PR-time feedback — an engine
contributor still will not see cron break in their own PR — and closing that gap
would require a check in `ossf/scorecard` itself, which is out of scope here and
worth revisiting only if the canary proves noisy in practice.

### C8 — Schema contract ownership after the split

`cron/internal/format` owns the published BigQuery/JSON schema contract
(`json.v2.schema`, `bq.raw.schema`, `json.raw.schema`), and `schema_gen_test.go`
verifies those schemas match Go structs in `checker`, `finding`, and
`docs/checks`. After the split the schema lives here while the data model it
serializes lives upstream, with nothing enforcing cross-repo sync at
engine-change time.

Note the duplication already exists: `pkg/scorecard/json_test.go` validates
against its own copy of `json.v2.schema`. The split makes an existing seam
visible rather than creating one.

**Decided: accept the drift risk, fail hard on it, and detect it on the engine's
cadence rather than the bump's.** Three parts:

- `schema_gen_test.go` continues to verify the published schemas against the
  engine data model, and a dependency bump that trips it **fails the bump** as a
  build break, never as a warning. A soft signal on a dependency PR is a signal
  that gets merged.
- The **C7 canary carries the schema check too** — it already builds and tests the
  pipeline against `ossf/scorecard`'s `main`, which runs `schema_gen_test.go`
  against the unreleased data model. One scheduled job answers both "did the
  engine break the pipeline" and "did the engine change the data model out from
  under the published schema". They are the same question asked of different
  files, and splitting them into two jobs would duplicate the setup for no
  additional coverage.
- Consolidating the two `json.v2.schema` copies stays a **separate, non-blocking
  track**. Making it a prerequisite would add an upstream change with its own
  review cycle in front of an extraction whose whole argument is that the coupling
  window is open now.

### C9 — Redirect stub upstream, not a clean delete

`projects.csv` is a high-traffic community contribution surface. Its
`ossf/scorecard` URL is linked from external documentation and bookmarked by
contributors who will not read a migration announcement.

A clean delete gives them a 404. A stub `cron/README.md` upstream pointing at the
new location gives them instructions. One file, retained indefinitely, against
months of misdirected PRs that someone has to triage and redirect by hand. Ship
the stub, plus explicit new URLs in `CONTRIBUTING.md` and an issue-template
callout.

### C10 — Cutover before deletion, with a full scan cycle in between

Phases are ordered so that the expensive-to-reverse step happens last and only
after the risky step has proven itself:

1. Build all six images here to a **staging tag or registry path** — never to the
   `:latest` / `:stable` tags that `cron/k8s/*.yaml` consume. Production keeps
   running upstream-built images while this repo's build is validated.
2. Diff a staging-built image against its production equivalent to confirm the
   split introduced no behavioral change.
3. Repoint the Cloud Build triggers in the `openssf` GCP project.
4. Run a full pipeline cycle end to end — controller → PubSub → worker → GCS →
   BigQuery transfer → webhook — and compare output row counts and schema against
   the prior cycle.
5. Hold for at least one full scan cycle.
6. Only then delete upstream.

Until step 6, **rollback is repointing the triggers back**. The code still exists
upstream, so there is nothing to restore. That property is the entire reason
deletion is last, and it is why step 5 must not be compressed for schedule.

### C11 — Convergence with the API server is deferred, deliberately

There is an obvious strategic question here: this repository already contains a
provider-agnostic read-through cache that writes Scorecard results to object
storage under a `{host}/{org}/{repo}[/{commit}]/results.json` key contract
(design **D3/D4**), and the incoming pipeline is a GCP-bound batch producer that
writes Scorecard results to a bucket. They are two halves of the same system.

Landing them in one repository is the precondition for reconciling them —
teaching the batch producer the store's key contract and backend abstraction
(`gocloud.dev/blob`) is how the GCP exit actually happens, and a locally-populated
store is what would let the orchestrator's cache tier serve batch-produced results
directly instead of reaching for `api.scorecard.dev` (**F8**). None of that is in
this change. Mixing a repository split with an architectural merge would make both
unreviewable, and the split has to be behavior-preserving to be safely revertible.

Two consequences for how this change is built, so the convergence stays cheap
later: keep the imported tree self-contained (no new imports from `cron/` into
`internal/`, and none the other way), and treat the pipeline's GCS write path as
the seam that will eventually be replaced by `internal/store`, not as something to
rewrite in passing.

The same logic applies to `/capabilities`: a server whose store is fed by this
repo's own batch pipeline has different caveats — different freshness, different
completeness, a different `publish_results` constraint — than one fed by live
scans or by the public upstream. Advertising that honestly is a follow-on change,
not a silent behavior shift bundled into a migration.

### C12 — A temporary CI guard against new inbound imports

The window between starting the extraction and deleting upstream is when someone
can add the first `.go` file outside `cron/` that imports `.../v5/cron/`, turning
a mechanical split into a refactor mid-flight.

A small CI check upstream that fails on any such import closes that window for the
cost of a few lines. It is added at extraction time and deleted along with `cron/`
itself, so it leaves no residue.

### C13 — Shared root build files are ported by hand, with recorded provenance

`filter-repo` selects content by path, not by hunk. Everything under `cron/` came
across with history — including all six Dockerfiles, all six Cloud Build configs,
and all twelve Kubernetes manifests, which is more of the deployment surface than
the phrase "port the build system" suggests. What could not come across is the
wiring that lives in files **shared with the scan engine**: the `Makefile` (96 of
467 lines are cron), `docker.yml`, `main.yml`, `dependabot.yml`, `.codecov.yml`,
and `cloudbuild/README.md`.

There is no path filter that extracts 96 lines from a 467-line file. The options
were to import those files whole — dragging ~370 lines of engine build targets and
the whole of Scorecard's CI along to recover a handful of recipes — or to port the
fragments by hand and lose their history. This repository has none of those files,
so a whole-file import would not have *collided*; the objection is semantic, not
mechanical.

**Decided: port by hand, and write down what the port erases.**
[`cron/initial-graft.md`](../../../cron/initial-graft.md) records the originating
commit for every ported target and job, the earlier lineage where the pickaxe is
misleading, and how to run further archaeology upstream. It sits **inside the
imported tree**, next to the code it explains, rather than in `docs/` or inside
this change: a provenance note filed where it gets archived defeats its own
purpose, and someone running `git log` on a ported Makefile target will be looking
at `cron/`, not at a docs index.

The loss is real but modest, and smaller than it is for `cron/`'s source. A build
recipe's history explains why a flag is set; it is not the load-bearing
operational record that makes `git blame` on a shard size or an ack deadline
valuable. Those 96 lines are also substantially rewritten in the port anyway — new
module path, new repository root, no shared engine variables — so their upstream
history describes a file that will not exist here. And `ossf/scorecard`'s history
stays public: deletion in Phase 6 removes the working tree, not the log.

Two things this decision turned up that the plan had wrong:

- **There are no cron `ko` configs to port.** Upstream's `.ko.yaml` declares a
  single `scorecard` build id and nothing cron-related. The "six ko targets" are
  Makefile recipes invoking `ko build` against import paths.
- **The C4 import rewrite is not confined to `.go` files.** Those ko recipes
  hardcode `github.com/ossf/scorecard/v5/cron/internal/worker`. The ported
  Makefile must use this module's path.

## Relationship to the existing API server

The imported pipeline does not touch the orchestrator, the store, the scan path,
or the HTTP surface. Stated explicitly against the areas where a reader would
reasonably expect interaction:

- **Read-through cache seam** (**D2/D5/D6**): unchanged. Freshness/TTL,
  single-flight, and the sync-vs-async decision are not consulted by the batch
  pipeline and do not learn about it. The pipeline is a producer with its own
  cadence; the orchestrator remains a request-driven consumer of `internal/store`.
- **Blob key contract and cloud-agnostic storage** (**D3/D4**): unchanged. The
  pipeline retains its own GCS write path and object layout, which is *not* the
  `{host}/{org}/{repo}[/{commit}]/results.json` contract. Reconciling the two —
  and putting `gocloud.dev/blob` underneath the pipeline — is the GCP-exit work
  this move enables (**C11**), and is where the "no hardcoded `gs://`" rule will
  eventually have to be applied to code that currently violates it. Until then,
  the pipeline's GCP coupling is quarantined inside `cron/`.
- **`/capabilities`** (the MCP hardcoded-caveat gap): unchanged in this change.
  The endpoint continues to describe only what the API server itself does.
- **Upstream graft map** (**D11**, `docs/upstream-graft.md`): the batch pipeline
  is explicitly **not** a graft target — it is arriving *from* upstream, not
  heading there. It is the one component in this repository whose direction of
  travel is inbound. `docs/upstream-graft.md` should say so, so a future reader
  does not try to graft it back.
- **`.golangci.yml`**: this repo's config is a near-verbatim port of upstream's,
  which is what makes porting 128 files plausible. Expect divergence anyway
  (`wrapcheck`'s module-name ignore list is the obvious one) and resolve it by
  adjusting the config or the code, never by blanket-disabling linters for the
  imported tree.

## Risks / trade-offs

- **`projects.csv` contributors land on a 404** — high likelihood, months of
  misdirected PRs. Mitigated by the redirect stub, `CONTRIBUTING.md` rewrite, and
  issue-template callout (**C9**). This is the risk most likely to be
  underestimated, because its cost is diffuse and lands on whoever triages issues.
- **Cloud Build triggers gate the cutover and are not in git.** Substantially
  reduced: the change author holds `openssf` project admin, so trigger repointing
  is not blocked on a third party's availability. What remains is that the
  triggers' configuration lives outside version control, so the cutover is not
  reviewable as a diff and not revertible by `git revert` — only by repointing
  them back (**C10**). Capture their before-state so the rollback is a known
  configuration, not a reconstruction.
- **External importers of `cron/data` or `cron/config` break.** Both are public
  upstream today. No consumers are known and none are presumed; Phase 0 verifies
  rather than discovers. If the check turns something up, upstream removal becomes
  a deprecation window rather than a delete — which is another reason `cron/`
  lands flat rather than under `internal/` (**C5**), since a deprecation window
  needs those packages to stay importable from somewhere.
- **Engine changes stop exercising the cron build at PR time.** Mitigated but not
  eliminated by the `main` canary (**C7**): breakage is caught within a scheduled
  interval rather than at the engine contributor's PR. The residual gap is real
  and is accepted.
- **Canary fatigue.** A scheduled job testing someone else's `main` will
  eventually go red for reasons unrelated to this repository. If it is ignored, it
  is worse than absent. See **C7** — route failures to a read channel and act on a
  sustained red.
- **Schema and data model split across repos with no compile-time guard.** See
  **C8**.
- **Longer CI here.** Six image builds and protobuf generation land in a
  repository whose CI is currently a Go build, a test run, and two linters. Worth
  measuring after Phase 4; if it becomes a problem, image builds are a candidate
  for a separate workflow rather than the presubmit path.
- **The repository's identity changes.** `AGENTS.md` currently describes this repo
  as an API server. After this it is an API server *and* the batch pipeline. That
  documentation debt is real and is part of the change, not a follow-up.

## Resolved during review

- **C4** — Tip-only import rewrite. Confirmed.
- **C5** — `cron/` lands at the top level; reorganization is a later, separate
  rename commit. Confirmed.
- **C6** — The token-pool server moves, to `cron/internal/githubserver/`. Upstream
  therefore builds no cron image at all after removal.
- **C7** — Pin to a release with an automated bump, **plus** a scheduled canary
  building the pipeline against `ossf/scorecard`'s `main`. Both halves, rather
  than trading reproducibility against feedback latency.
- **C8** — Accept cross-repo drift, fail the dependency bump hard on it, and fold
  the schema check into the C7 canary so drift surfaces on the engine's cadence.
  Schema consolidation stays a separate, non-blocking track.
- **C9** — Redirect stub at `cron/README.md` upstream, retained indefinitely, plus
  the `CONTRIBUTING.md` rewrite and issue-template callout.
- **Cloud Build triggers** — The change author holds `openssf` project admin;
  cutover is not blocked on third-party availability. Scheduling the cutover
  window is the remaining constraint.
- **Scope** — Upstream removal (groups 6–7) stays inside this change rather than
  splitting into a separate artifact, because the **C10** rollback contract spans
  both repositories and would drift if tracked separately.
- **C13** — Shared root build files are ported by hand rather than imported whole,
  with their provenance recorded in `cron/initial-graft.md`. Raised during
  extraction, not during the original review.

## Open questions

- **External importers.** No consumers of
  `github.com/ossf/scorecard/v5/cron/data` or `.../cron/config` are known, and the
  working assumption is that there are none. Phase 0 verifies this (GitHub code
  search plus `pkg.go.dev` importers) rather than leaving it to be discovered at
  removal time. A positive result changes upstream removal from a delete to a
  deprecation window.
- **Canary failure routing.** Where a red `main` canary should page or post is an
  operational detail to settle when the job is built (task 4.7), not a blocker.
- **Steering Committee format.** This change is the authoritative artifact. Whether
  the committee wants a standalone summary document rather than an OpenSpec change
  is worth asking before the approval request goes out.
