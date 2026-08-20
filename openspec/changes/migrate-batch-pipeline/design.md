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

Leaving it upstream is a legitimate alternative — it costs upstream one
indefinitely-maintained cron image. Moving it is the recommendation; it needs
explicit confirmation because it is the one path outside `cron/` that this change
touches.

### C7 — Pin the engine dependency to a release, with an automated bump

Today the cron worker builds against in-tree scorecard, so an engine change that
breaks cron fails at PR time. After the split it builds against whatever `go.mod`
pins (`v5.5.0` currently).

- **Pin to a release** (recommended), with a scheduled job or Dependabot rule
  bumping to the latest tag. Production scans then run a known engine version,
  which is arguably more correct than today's implicit tracking of `main`.
- **Track `main`** via a pseudo-version. Preserves the tight feedback loop, at the
  cost of non-reproducible production builds.

The trade-off is real and unavoidable: under the recommended option, an engine
change that breaks cron surfaces at bump time rather than at PR time. Whoever
owns cron operations makes this call — it is an operational preference, not a
technical constraint.

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

Accept the drift risk for now: `schema_gen_test.go` still catches mismatches, but
at dependency-bump time rather than at engine-PR time. Consolidating the two
`json.v2.schema` copies is tracked separately and is not a blocker. The decision
that matters here is how loudly the dependency bump should fail — it should fail
hard, as a build break, not as a warning.

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
- **Cloud Build triggers gate the cutover and are not in git.** If the person with
  `openssf` project admin is unavailable, the migration stalls indefinitely at the
  last reversible step. Identified and scheduled before any code moves — this is
  the reason Phase 0 exists as a distinct phase.
- **External importers of `cron/data` or `cron/config` break.** Both are public
  upstream today. Checked in Phase 0; if any consumer exists, upstream removal
  becomes a deprecation window rather than a delete.
- **Engine changes stop exercising the cron build at PR time.** Inherent to the
  split, not to how it is done. See **C7**.
- **Schema and data model split across repos with no compile-time guard.** See
  **C8**.
- **Longer CI here.** Six image builds and protobuf generation land in a
  repository whose CI is currently a Go build, a test run, and two linters. Worth
  measuring after Phase 4; if it becomes a problem, image builds are a candidate
  for a separate workflow rather than the presubmit path.
- **The repository's identity changes.** `AGENTS.md` currently describes this repo
  as an API server. After this it is an API server *and* the batch pipeline. That
  documentation debt is real and is part of the change, not a follow-up.

## Open questions

- **C6:** Confirm the token server should move at all, and confirm
  `cron/internal/githubserver/` as its destination.
- **C7:** Pin-to-release or track-`main`? Needs the cron operations owner.
- **C8:** Should the dependency bump fail hard on schema drift, and is
  consolidating the duplicate `json.v2.schema` a prerequisite or a follow-on?
- **Phase 0:** Who holds Cloud Build trigger edit rights in the `openssf` GCP
  project, and are they available for the cutover window?
- Are there known external consumers of `github.com/ossf/scorecard/v5/cron/data`
  or `.../cron/config`?
