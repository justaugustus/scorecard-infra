# Design: Migrate the results API out of ossf/scorecard-webapp

## Context

`ossf/scorecard-webapp` hosts two deployable projects that share a repository and
nothing else. `scorecards-site/` is a Nuxt application published to Netlify.
`app/` plus a root-level entrypoint, contract, and build wiring is a Go REST API
published to Cloud Run behind Google Cloud Endpoints — the service answering
`api.scorecard.dev` and `api.securityscorecards.dev`.

Three properties of the current state determine the design.

**The coupling is a deployed URL, not a build edge.** Nothing in the site's
JavaScript imports Go, and nothing in the Go imports the site. The site's result
viewer fetches `https://api.scorecard.dev/projects/...` at runtime. So the split
severs no build dependency — but it leaves the site a *client* of the moved
service, which is one more reason the contract must not shift during the move.

**The history is clean to extract, but the paths are not namespaced.** Of 150
commits touching the API, exactly one also touches the site: a 2022 project-wide
copyright rename. Every historical rename in the API's lineage has both endpoints
inside the API set (`app/*.go` → `app/server/*.go` in the 2022 go-swagger
adoption; `app/signing_test.go` → `app/post_results_test.go`), so a path filter
cannot truncate a file's history. What differs from the batch pipeline's
migration is that six of the API's files live at the webapp's repository root,
three of which — `Makefile`, `Dockerfile`, `.golangci.yml` — already exist here.

**This repository already contains the rest of the data path.** `cron/` writes
`gs://ossf-scorecard-results`. The imported API reads it. `internal/store` reads
the same key contract through `gocloud.dev/blob` against a configurable backend.
That is the reason to do this, and simultaneously the largest hazard in doing it:
after the graft, one repository holds two HTTP servers that answer the same
contract, and the temptation to merge them mid-migration will be strong.

The current priority settles which one wins in the meantime. This is a
**lift-and-shift** — re-host running infrastructure without changing it — so the
imported implementation is the preferred one and the one that gets redeployed,
and `internal/httpapi` stays in place but off the deployment path (**W10**).

The hard parts are therefore not the Go code. They are the shared root files,
the generated-code toolchain, the external systems that are configured outside
git — Cloud Build, Cloud Run, Cloud Endpoints, DNS, and OSS-Fuzz — and holding
the line on behavior freeze while the obvious refactor sits in the same tree.

## Goals / Non-Goals

**Goals:**

- Land the API here with history, blame, and rename tracking genuinely intact.
- Change no API behavior. Behavioral equivalence between the upstream-deployed
  and infra-deployed service is a gate, not an aspiration — the website, every
  published badge, the Scorecard Action's upload path, and `scorecard-mcp` are
  all live consumers.
- Keep rollback cheap for as long as possible: a redeploy of the previous
  revision, not a code restoration.
- Leave `ossf/scorecard-webapp` a coherent website repository rather than a
  half-emptied one.
- Redeploy the imported service as-is, and leave `internal/httpapi` in place but
  off the deployment path, without deleting it (**W10**).
- Keep each phase independently reviewable and independently revertible.

**Non-Goals** (see proposal): behavior or contract changes, removing the GCP
coupling, reconciling the imported API with `internal/`, restructuring into
`internal/` + `cmd/`, moving the website, and choosing where production runs.

## Decisions

**Status:** W1–W16 are proposed; none is implemented. W10 is the one a reviewer
should push on hardest — it is where this migration diverges from the batch
pipeline's, and where an approval commits the repository to a direction rather
than to a mechanical move. W14 is the one whose cost compounds silently.

### W1 — `git filter-repo` against a disposable clone, with mandatory renames

Same tool and same reasoning as the batch pipeline's **C1**: `filter-repo`
rewrites the commit graph to contain only selected paths while preserving
authorship, dates, messages, and rename detection. It runs against a fresh
single-branch clone, never a working checkout.

What differs is that every path must be renamed on the way out, because the
API's files are not namespaced upstream:

```bash
git clone --single-branch --branch main \
  https://github.com/ossf/scorecard-webapp.git scorecard-api-extract
cd scorecard-api-extract
git tag -l | xargs -r git tag -d                    # W2
git filter-repo \
  --path app/ \
  --path main.go \
  --path openapi.yaml \
  --path Makefile \
  --path Makefile.swagger \
  --path COPYRIGHT.txt \
  --path Dockerfile \
  --path docs/dns.md \
  --path-rename app/:api/app/ \
  --path-rename main.go:api/main.go \
  --path-rename openapi.yaml:api/openapi.yaml \
  --path-rename Makefile:api/Makefile \
  --path-rename Makefile.swagger:api/Makefile.swagger \
  --path-rename COPYRIGHT.txt:api/COPYRIGHT.txt \
  --path-rename Dockerfile:api/Dockerfile \
  --message-callback '
      return re.sub(rb"\(#([0-9]+)\)", rb"(ossf/scorecard-webapp#\1)", message)
  '                                                  # W3
```

Two mechanical cautions, both verifiable on the dry run rather than trusted:

- **`--path` matches exact paths and directory prefixes — not string prefixes.**
  An earlier draft of this section asserted the opposite, and left
  `Makefile.swagger` out of the list on the theory that `--path Makefile` would
  select it. It does not: the first extraction produced 116 files instead of 117,
  and `Makefile.swagger` was the one missing. Every non-directory path needs its
  own `--path` and its own `--path-rename`. **The file-count gate is what caught
  this**, which is the argument for stating expected counts as gates rather than
  as estimates.
- **`docs/dns.md` is deliberately not renamed.** It lands at `docs/dns.md` in
  this repository, where there is no collision, because it documents this
  repository's DNS rather than the imported tree's internals (**W6**).

Grafting uses `git merge --allow-unrelated-histories`, conflict-free by
construction: nothing here occupies `api/` or `docs/dns.md`.

### W2 — Strip the release tags before the graft

The webapp carries 20 `v1.0.x` tags. Two independent reasons to delete them
before merging, either of which would be sufficient: they would pollute this
repository's release namespace, and they *mean something operationally* —
upstream's release process is "push a tag, the production API deploys." Importing
a tag whose semantics are "deploy the API" into a repository that does not yet
deploy the API is worse than a naming collision.

### W3 — Rewrite PR references during the filter

140 of 150 commit subjects end in `(#N)`. GitHub auto-links those to the
containing repository, so after the graft each would resolve to an unrelated item
here. Rewriting to `(ossf/scorecard-webapp#N)` during the filter makes them
resolve correctly and permanently. Fixing this afterwards means rewriting grafted
history in a repository others have pulled.

### W4 — Rewrite paths at the tip, and enumerate the non-Go references first

Rewriting `github.com/ossf/scorecard-webapp/app/...` across 150 commits would
make each independently buildable and would also be false — those commits
genuinely contained the old module path. One tip commit is honest, reviewable as
a diff, and costs nothing: `git blame` follows content regardless.

The batch pipeline's migration learned the expensive half of this at image-build
time: relocation moves files but not the paths written *inside* them, and Go is
the only one of those languages that fails loudly. The API's non-Go references
are therefore enumerated up front:

| Reference | Where | Breaks when |
| --- | --- | --- |
| `ADD . $APP_ROOT/src/` + `make scorecard-webapp` | `Dockerfile` | Image build — the module root is now the repository root, two levels up from the build target |
| `main.go app/server/*.go` prerequisites, `-o scorecard-webapp` | `Makefile` | `make` from the wrong directory silently rebuilds nothing |
| `swagger generate ... -t app/generated`, `-f openapi.yaml`, `-r COPYRIGHT.txt` | `Makefile` | Regeneration writes to the wrong tree |
| `find app/generated/... ` in the `SWAGGER_GEN` recipe | `Makefile` | `make clean` deletes nothing, or the wrong thing |
| `//go:generate swagger generate server --target ../../generated --spec ../../../openapi.yaml` | `app/generated/restapi/configure_scorecard.go` | `go generate` writes outside the tree |
| `-ignore "app/server/testdata/**" -ignore "app/generated/**"` | `addlicense` invocation | License check starts failing on generated and fixture files |

`//go:embed static` in the same file is path-relative and survives the move; it
is listed here only so a reviewer does not have to work that out.

### W5 — Land under `api/`, upstream layout intact; reorganize later, separately

The imported tree lands under a single top-level `api/`, alongside `cron/`, with
its internal layout untouched: `api/app/server/`, `api/app/generated/`,
`api/main.go`, `api/openapi.yaml`.

The name is the plain one on purpose. A provenance name like `webapp/` would
mirror how `cron/` records where it came from, but it names a thing this tree is
not — the actual web application stays upstream — and it frames the import as a
guest at exactly the moment the repository stops treating it as one (**W10**).
The apparent collision with the existing `api-server` capability and
`cmd/scorecard-api` resolves the same way: the imported code *is* what serves
`api.scorecard.dev` today, deployed as the Cloud Run service named
`scorecard-api-prod`. The incubator borrowed the name; the import brings the
thing the name refers to.

The redundant `app/` level is kept rather than flattened. `--path-rename
app/:api/` would be exactly as mechanical as `app/:api/app/` and produces a
tidier tree, but it is a structural opinion baked into a history import, and it
breaks the 1:1 path correspondence with upstream during the window where both
trees exist and are being diffed against each other (**W11**). Flatten it later,
in the same separate reorganization commit that **C5** already defers the
`internal/` + `cmd/` question to.

**Binary names stay as they are through cutover.** `go build ./...` will produce
two servers answering overlapping routes: `scorecard-api` from `cmd/`, and
`scorecard-webapp` from `api/`. The names invert reality — the "webapp" one is
the production API — and fixing that is tempting because the `Dockerfile` is
being rewritten anyway (**W4**). Resist it until cutover completes: renaming the
built binary changes the contents of the image whose equivalence to production is
the cutover gate. Rename in the reorganization commit, when nothing depends on
sameness.

### W6 — The shared DNS document is imported whole and trimmed at both ends

`docs/dns.md` is the one file in the extraction set that genuinely serves both
projects: it documents the Cloud DNS zones for `scorecard.dev` and
`securityscorecards.dev`, the Netlify certificate setup for the site, and the
Cloud Run domain mapping for the API.

A path filter cannot split a file. The options were to leave it upstream and
rewrite the API half from memory here, or to import it whole and delete the site
half at the tip. Importing it whole is better: both halves keep their history,
and upstream's deletion of the API half is an ordinary reviewable diff rather
than a silent divergence. The cost is a brief window where two repositories
document the same DNS zones — closed in Phase 7.

### W7 — The linter configuration is reconciled by hand, not imported

`golangci-lint run ./...` reads one configuration, the nearest to the working
directory. An `api/.golangci.yml` would therefore be a dead file — present,
plausible-looking, and ignored. The webapp's config is excluded from the
extraction set and reconciled by hand into this repository's root config, with
its provenance recorded in `api/initial-graft.md`.

Expect real divergence rather than a clean union. This repository's config is a
near-verbatim port of `ossf/scorecard`'s; the webapp's is its own lineage, and
the imported generated tree will need exclusions that hand-written code should
not get. Resolve conflicts by narrowing exclusions to the generated paths — never
by disabling a linter repository-wide to accommodate the import.

### W8 — CI is hand-ported into existing workflows; the fuzzing integration follows the code

The batch pipeline's **C13** hand-ported build wiring because `filter-repo`
selects by path and the wiring lived in engine-shared files. Here the constraint
is different — upstream's `main.yml` is entirely API content and *could* be
imported — but the conclusion is the same, for a second reason: this repository
already has `presubmits.yml` doing build, test, and lint. Importing a second
workflow that does the same three things on the same triggers produces duplicate
required checks, not coverage.

What gets ported, therefore, is the delta:

- the `addlicense` header check, which this repository does not have;
- a GitHub token in the test job's environment (**W9**);
- a go-swagger regeneration check, so a hand-edit to a generated file is caught;
- one image build target added to `build-images.yml`, alongside the pipeline's six.

`cifuzz.yml` is a different case. It is not duplicated here, and it is not
optional: the `scorecard-web` OSS-Fuzz project fuzzes two targets in
`app/server/`, and its build configuration in `google/oss-fuzz` names the source
repository. The workflow is ported and the OSS-Fuzz project is repointed as part
of cutover. Forgetting this does not fail a build — fuzzing simply stops finding
things — so it is a Phase 6 gate rather than a Phase 4 nicety.

`codeql-analysis.yml` is not ported: its Go half duplicates scanning this
repository already performs, and its JavaScript half belongs to the site.

### W9 — Network-dependent tests keep running; the token is supplied, not the tests skipped

The imported Ginkgo end-to-end specs verify real Sigstore certificates against
live Rekor and resolve commits against the live GitHub API. They already handle
the one flake that is not a real failure — Rekor's search-by-hash index, removed
in Rekor v2, raises a typed error that the specs skip on. The rest need a GitHub
token; upstream supplies `github.token`.

This repository's test job supplies neither a token nor, currently, any
tolerance for network-dependent tests. The available options were to gate the
specs behind a build tag, to skip them when the token is absent, or to supply the
token. **Supply the token.** Gating them means the verification logic behind the
publish path — the part of this API with actual security consequences — stops
being exercised the moment it moves, and a suite that silently no-ops is worse
than an absent one. If they prove flaky in practice, the fix is narrowing the
skip conditions to specific typed errors, as the Rekor guard already does, not
widening them.

**Verified (PR #49): 8 specs pass, 0 skip, with the token supplied.** Worth
recording how nearly this went the other way. In the authoring environment the
Sigstore specs all skipped, because that network returns an HTML `403` from every
Rekor endpoint — and read alongside the upstream guard's comment about Rekor v2
removing the search index, that looked like proof the coverage was gone
everywhere. It was a local proxy block. The conclusion drawn from it would have
justified treating the publish path as untested and skipping work accordingly.
The general lesson for this migration: a skip observed on one network is a
reachability signal, not a property of the test suite.

### W10 — Lift-and-shift: the imported server is the one that ships

This is the decision the rest of the change rests on, and it is where this
migration stops resembling the batch pipeline's. After the graft this repository
will contain two implementations of one contract:

- `api/app/server/get_results.go` — reads `gs://ossf-scorecard-results`, falls
  back to `gs://ossf-scorecard-cron-results`, and is the code currently serving
  `api.scorecard.dev` in production;
- `internal/store` + `internal/httpapi` — reads a configurable
  `gocloud.dev/blob` backend under the same key contract, a deliberate
  incubator-built mirror of that same GET contract that has never served
  production traffic;

with `cron/` writing the bucket the first one reads.

**The imported implementation is preferred, and it is what gets redeployed.**
The immediate objective is to re-host running infrastructure without changing it
— a lift-and-shift — not to arrive at the better architecture. Between two
implementations of one contract, the one with production behavior already proven
against every live consumer wins by default, and it wins *especially* during a
migration whose acceptance test is that nothing changed.

That leaves `internal/httpapi` and its supporting packages in place, unbuilt
against, and not on the deployment path. Not deleted: the decision is explicitly
revisitable, the packages are what the provider-agnostic work was learned in, and
`internal/store`'s configurable-backend seam is the most likely shape of the
eventual GCP exit. But nobody should read the two servers as peers awaiting a
merge on the merits. One is running; the other is a study.

Three constraints follow, and they should be read as prohibitions for the
duration of the freeze:

- **No import edges** between `api/` and `internal/` in either direction, and
  none between `api/` and `cron/`. CI enforces this the same way it does for the
  pipeline. This is not a statement that the two servers are equals — it is what
  keeps the imported tree byte-comparable to what production runs.
- **The hardcoded `gs://` constants stay.** This repository's cloud-agnostic
  rules are non-negotiable *for this repository's own code*; the imported tree is
  behavior-frozen until cutover completes, exactly as `cron/`'s GCS write path
  is. A future agent reading `AGENTS.md` will find three rule violations in
  `api/` and want to fix them. `AGENTS.md` must say, in the imperative, that they
  are quarantined and why.
- **`openapi.yaml` is not edited.** It is simultaneously the published contract
  and the Cloud Endpoints deployment configuration, including an
  `x-google-backend` address. Changing it changes a deployed service.

**D11 is not inverted, it is retired.** `docs/upstream-graft.md` describes this
repository as an incubator whose durable pieces graft outward — `internal/store`
and the `/projects` handlers to `ossf/scorecard-webapp`, the live-scan path to
`scorecard serve`. Two of those three targets are now here, and the code they
were meant to improve on arrived with them. The document is rewritten, not
amended, around what the repository has actually become: the unified home for
Scorecard's infrastructure, where consolidation happens by moving code in rather
than by grafting it out.

Two consequences to record while rewriting it, because they are rationales that
quietly stopped holding rather than decisions anyone made:

- `internal/model` (**D13**) was justified partly by the webapp's generated
  models living upstream. They do not any more.
- What remains genuinely graftable upstream is narrower than D11 claims —
  realistically the `/capabilities` endpoint (**D7**) and whatever of the
  orchestrator survives, and only once there is one server here rather than two.

The endgame is unchanged and worth restating so that preferring the imported code
does not read as abandoning the rest: one server, serving the published contract,
reading a configurable backend, optionally scanning live, fed by a producer that
writes through the same store. This change assembles the parts in one place and
keeps the lights on. A later change makes them one thing — and that change starts
from the code that is running, not from the code that was designed.

### W11 — Cutover before deletion, with a response diff and a hold period

Ordered so the expensive-to-reverse step happens last:

1. Build the image here to a **staging tag** — never the tag the production Cloud
   Run service consumes.
2. Deploy it to a **non-production Cloud Run revision** with no traffic assigned.
3. **Diff responses** between the staging revision and production across a fixed
   request set: a known-good repository, a repository with no results, a pinned
   commit, a malformed request, and each badge style. Compare status codes,
   bodies, and the `Cache-Control` / `Surrogate-Control` headers — the cache
   headers are load-bearing for Fastly and are the easiest thing to regress
   invisibly.
4. Repoint the Cloud Build trigger, shift traffic, and repoint the OSS-Fuzz
   project.

   **Corrected after capturing the deployment (task 6.3c).** "Shift traffic"
   here is a **Fastly backend change, not a DNS repoint**. Both API hostnames
   are `CNAME`s to `x.sni.global.fastly.net`; the origin lives in Fastly
   configuration, and no Cloud Run domain mapping is in the path. That makes
   Fastly the cutover control plane — and it is the one system in the inventory
   `gcloud` cannot reach, so its configuration has to be captured by hand before
   anything moves. It also makes rollback faster than this section assumed: flip
   the backend back, with no propagation to wait out.
5. Hold. Watch error rates, the badge path, and at least one Scorecard Action
   upload completing end to end through the `POST` path.
6. Only then delete upstream.

Until step 6, **rollback is shifting traffic back to the prior revision.** The
code still exists upstream, so there is nothing to restore. Capture the current
Cloud Build, Cloud Run, and Cloud Endpoints configuration before changing any of
it, so the rollback is a known state rather than a reconstruction.

The `POST` path deserves particular attention at step 5 in a way `GET` does not:
a broken read is visible immediately and fixed by a rollback, while a broken
publish path silently stops accepting results and is noticed days later as
staleness.

### W12 — Upstream keeps the website; state what is retired

Unlike the pipeline's migration, no repository is emptied. `ossf/scorecard-webapp`
remains the website's home, which makes the removal simpler — there is a
maintained `README.md` to rewrite rather than a bare 404 to apologize for.

What removal actually retires is the Go module. Deleting `app/`, `main.go`, and
the build wiring leaves no Go code, so `go.mod` and `go.sum` go too, and
`github.com/ossf/scorecard-webapp` stops resolving at `@latest` or `@main`.

**There is one external consumer, and it survives.** `jetstack/tally` imports
`app/generated/models` from four of its packages — the predicted consumer,
found by task 0.1. Nothing imports the generated client, the `restapi` package,
or the module root. What makes this a non-blocker is *how* tally depends on it:
`require github.com/ossf/scorecard-webapp v1.0.5`, a released version. The Go
module proxy holds all 20 published versions immutably and independently of the
repository's working tree, so a pinned build keeps resolving after the code is
deleted. Removal breaks `@latest` resolution and new consumers, not existing
pinned ones. tally is also dormant — last pushed 2023-11-13, still on
`scorecard/v4`.

This adds exactly one obligation, and it is a prohibition rather than work:
**upstream's release tags must not be deleted.** Deleting the code is safe;
deleting `v1.0.5` is what would actually break someone. Worth stating explicitly
because "clean up the repository" plausibly includes pruning tags that no longer
trigger anything, and because this change strips those same tags from the
*import* (**W2**) — the two operations look similar and have opposite
consequences.

A deprecation window is therefore not needed. The tree still lands importable at
`api/` rather than under `internal/` (**W5**), which keeps the option open if a
consumer surfaces later.

### W13 — DCO versus imported history, again

37 of the 150 imported commits carry no `Signed-off-by` trailer. This repository
gates pull requests on DCO, and the check inspects every commit in the pull
request, so the import fails DCO by construction.

There is no code fix. A DCO trailer is a legal certification by the commit's
author; adding one retroactively on someone else's behalf forges it. It is also
unnecessary — these commits are already public in `ossf/scorecard-webapp` under
Apache-2.0, and importing them does not change their provenance.

The batch pipeline's import hit this at merge time and resolved it
administratively. Here it is known in advance, so the resolution should be agreed
*before* review opens rather than discovered in it. Same two viable paths:
an administrative override, or scoping the DCO app's check.

### W14 — Three roots in one repository

After this graft, `ossf/scorecard-infra` has three independent history roots: its
own, the batch pipeline's (2020), and the API's (2021). `git log` for the
repository as a whole becomes an interleaving of three projects' histories, and
bisecting across the whole tree becomes meaningless.

This is a real cost and it is accepted, because the alternative is worse in
proportion: squashing either import to keep `git log` tidy destroys exactly the
operational record the imports exist to preserve. The mitigations are
conventional — read history per-path (`git log -- api/`), and treat
whole-repository `git log` as a merge of three streams rather than a narrative.

It is worth noticing, though, that this is the second time; a third import should
prompt the question of whether the repository is accumulating a monorepo by
default rather than by decision.

### W15 — Inherit the release trigger at cutover; decouple build from deploy after

Upstream releases the production API by pushing a `v1.x` tag, which fires a Cloud
Build trigger that builds the image and deploys it to Cloud Run; `openapi.yaml`
changes on `main` reach staging only, and the Cloud Endpoints service
configuration is deployed by hand (upstream's own README calls this a TODO). This
repository, by contrast, has no tags at all: `cron/`'s images are built by
repository-scoped Cloud Build triggers, and CI builds images without publishing
them. So there is no existing convention here to inherit into.

The complication is that the obvious inheritance deepens exactly the coupling
this repository exists to remove. Recommended sequence:

**1. At cutover, inherit the mechanism unchanged, repointed at this repository.**
The cutover already changes where the source lives and who builds the image;
changing how a release is triggered in the same step means a failure has two
candidate causes instead of one. Lift-and-shift applies to the release path too.

**2. Scope tags to the component from the first one: `api/vX.Y.Z`, not
`vX.Y.Z`.** This repository now holds two independently deployable systems and
will likely hold more. A bare `v1.2.3` here would claim the whole repository for
one component, collide with any future module-level versioning of
`scorecard-infra` itself, and read ambiguously next to upstream's `v1.0.x`, which
must keep resolving for the one external consumer (**W12**). Component-scoped
tags are also what Go's own submodule convention expects, so the namespace stays
usable if `api/` ever becomes its own module. This is the one piece worth
deciding *before* the first tag, because renaming a release namespace afterwards
is worse than choosing it awkwardly now.

**3. After the hold period, split building from deploying.** Move image *build
and publish* into GitHub Actions, producing a digest-addressed image in a
registry; leave the provider-specific step as "deploy this digest." That is the
portability seam. It shrinks the GCP-specific surface from a build system plus a
deploy system to a single deploy step, and it makes the artifact — an OCI image
identified by digest — the thing that moves between providers, rather than a
build configuration that has to be reimplemented against each one.

**4. Do not adopt deploy-on-merge for the API.** The pipeline's images can
rebuild on whatever cadence suits them; a public API serving badges and result
lookups should have an explicit, nameable release. Keeping the trigger a
deliberate act is also what makes step 3's digest pinning meaningful.

A corollary worth recording even though it is out of scope: deployments should
consume immutable digests rather than floating tags. `cron/k8s/*.yaml` currently
consumes `:stable`, which means a rollback there is not a configuration change
with a known target. That is a pre-existing weakness in the pipeline, not
something this change introduces or fixes, but the API's cutover contract
(**W11**) depends on being able to name a prior artifact exactly — so the API
should start out doing it properly rather than inheriting the habit.

### W16 — Regeneration matches the checked-in tree, and the version must be recovered

Between pinning the generator to whatever produced the checked-in generated tree
and regenerating with a current go-swagger, **match the checked-in tree.** A
migration whose acceptance test is "nothing changed" should not carry a few
hundred lines of generator modernization through it, and toolchain currency is a
real concern that deserves its own change rather than a ride on this one.

The awkward part is that the version is not recorded anywhere. The generated
files carry `// Code generated by go-swagger; DO NOT EDIT.` and no version, and
the webapp's `Makefile` invokes whatever `swagger` happens to be on `PATH`. This
is the same unpinned-tool problem the pipeline import hit with `protoc`, and it
has the same consequence: the checked-in tree's provenance is whatever was
installed on the last regenerating machine.

So the decision has a prerequisite (task 0.4a): bisect go-swagger releases around
2026-02-05 — the last commit to touch the generated tree — until `make swagger`
produces no diff, then pin that version somewhere CI enforces. If no release
reproduces the tree exactly, that is a finding to escalate rather than absorb: it
means the checked-in output cannot be reproduced by any released tool, and the
choice becomes regenerate-current-and-accept-the-diff with that fact stated.

## Relationship to the existing API server and pipeline

Stated explicitly against the areas where a reader would reasonably expect
interaction:

- **`internal/httpapi`** — unchanged in code, changed in status. It keeps
  building, keeps its tests, and stops being the forward path: the imported
  server is what deploys (**W10**). Neither learns about the other. Do not
  develop new API surface here on the assumption it will be the one that ships.
- **`internal/store`** (**D3/D4**) — unchanged. The imported API keeps its own
  direct `gocloud.dev/blob` calls with hardcoded bucket URLs. Putting the store
  underneath it is the convergence work this move enables (**W10**), not part of
  the move — and the direction of that work is now "teach the running server the
  configurable backend", not "adopt the incubator's server".
- **`internal/model`** (**D13**) — unchanged, but its rationale weakens: it was
  justified partly by the webapp's generated models living upstream, and they no
  longer will. Revisit in the convergence change, not here.
- **`/capabilities`** (**D7**) — unchanged. It continues to describe only the
  server it belongs to.
- **`cron/`** (**C11**) — unchanged, and specifically *not* wired to the imported
  API even though the API reads the bucket the pipeline writes. That linkage is
  the most tempting one in the repository and the most important to leave alone
  until both trees are out of behavior freeze.
- **`docs/upstream-graft.md`** (**D11**) — rewritten, not amended. See **W10**.

## Risks / trade-offs

- **Two servers answering one contract in one repository.** The failure mode is
  not a bad merge; it is a well-intentioned tidy-up. Mitigated by an explicit
  prohibition in `AGENTS.md`, a CI import-edge check, and this document — the
  same three mechanisms the pipeline's **C11** uses, which have held so far.
- **A deprioritized package rots quietly.** `internal/httpapi` keeps compiling
  and keeps passing tests while nothing depends on it, which is exactly the state
  in which code stops being true without anything going red. Accepted for now
  (**W10**); the honest options at the next decision point are to converge it or
  to delete it, not to leave it indefinitely as a maintained study.
- **The website is a live client of the moved service.** Any contract drift
  during the move breaks a page nobody thought to test. The response-diff gate
  (**W11**, step 3) is the mitigation; the site's viewer path should be in the
  fixed request set.
- **The generated tree's provenance is unrecorded.** Settled in favour of
  matching the checked-in output (**W16**), which leaves the question of which
  go-swagger produced it — a question nobody wrote down and CI never enforced.
  The residual risk is that no released version reproduces it, in which case the
  tree cannot be regenerated by anyone and the decision reopens.
- **`configure_scorecard.go` is hand-owned inside a generated tree.** It is
  excluded from `SWAGGER_GEN` and holds the handler wiring, the CORS setup, and
  the JSON producer configuration. A regeneration that overwrites it silently
  reverts routing. The regeneration check in **W8** must assert it is unchanged.
- **`openapi.yaml` is contract and deployment configuration at once.** There is
  no way to change one without touching the other, which is why it is frozen
  here. Long-term this is a design problem the convergence change inherits.
- **External systems, none in git.** Cloud Build trigger, Cloud Run service and
  revisions, Cloud Endpoints service configuration, domain mapping, Fastly, and
  the OSS-Fuzz project. Capture before-state for each (**W11**).
- **Network-dependent tests in the presubmit path** (**W9**). Accepted
  deliberately; the alternative silently drops coverage of the verification
  logic.
- **DCO** (**W13**) — known in advance this time.
- **Three history roots** (**W14**).
- **The repository's identity changes again.** `AGENTS.md` currently describes
  three parts; it will describe four, one of which duplicates another. That
  documentation debt is part of this change, not a follow-up.

## Open questions

- **Where production runs after cutover.** The destination for the hosted API is
  being settled outside this change. Phases 0–5 are independent of that outcome;
  Phase 6 is sequenced against it, and Phase 7 must not begin until it is
  settled.
- **Which go-swagger version produced the checked-in tree.** **W16** decided to
  match it; nobody recorded what it was. Recovering it is task 0.4a, and the
  answer could be "no release reproduces it exactly", which reopens **W16**.
- **Steering Committee artifact format.** This change is the authoritative
  artifact. Whether the committee wants a standalone summary instead is worth
  asking before the approval request goes out — the same question the pipeline's
  migration left open.

## Resolved during review

- **External importers (W12).** One found: `jetstack/tally` imports
  `app/generated/models` from four packages. It pins the released version
  `v1.0.5`, which the module proxy holds immutably, so upstream removal does not
  break it — provided upstream's tags survive. Clean delete, no deprecation
  window, one new obligation. See **W12**.
- **Generator pinning (W16).** Match the checked-in tree, not toolchain currency.
- **DCO (W13).** Administrative override, agreed in advance.
- **Release triggers (W15).** Inherit unchanged at cutover, decouple build from
  deploy after the hold.
