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
- Keep each phase independently reviewable and independently revertible.

**Non-Goals** (see proposal): behavior or contract changes, removing the GCP
coupling, reconciling the imported API with `internal/`, restructuring into
`internal/` + `cmd/`, moving the website, and choosing where production runs.

## Decisions

**Status:** W1–W14 are proposed; none is implemented. W10 and W14 are the two
that a reviewer should push on hardest — they are where this migration diverges
from the batch pipeline's, and where an approval commits the repository to
something structural.

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
  --path COPYRIGHT.txt \
  --path Dockerfile \
  --path docs/dns.md \
  --path-rename app/:webapp/app/ \
  --path-rename main.go:webapp/main.go \
  --path-rename openapi.yaml:webapp/openapi.yaml \
  --path-rename Makefile:webapp/Makefile \
  --path-rename COPYRIGHT.txt:webapp/COPYRIGHT.txt \
  --path-rename Dockerfile:webapp/Dockerfile \
  --message-callback '
      return re.sub(rb"\(#([0-9]+)\)", rb"(ossf/scorecard-webapp#\1)", message)
  '                                                  # W3
```

Two mechanical cautions, both verifiable on the dry run rather than trusted:

- **`--path` and `--path-rename` match by prefix, not by exact filename.**
  `--path Makefile` also selects `Makefile.swagger`, and
  `--path-rename Makefile:webapp/Makefile` also relocates it to
  `webapp/Makefile.swagger`. That is the desired outcome here, and it is the
  reason `Makefile.swagger` does not appear in the list — but the same property
  would silently pull in an unrelated `Dockerfile.debug` if one existed. The file
  count gate (117) is what catches a mistake.
- **`docs/dns.md` is deliberately not renamed.** It lands at `docs/dns.md` in
  this repository, where there is no collision, because it documents this
  repository's DNS rather than the imported tree's internals (**W6**).

Grafting uses `git merge --allow-unrelated-histories`, conflict-free by
construction: nothing here occupies `webapp/` or `docs/dns.md`.

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

### W5 — Land under `webapp/`; reorganize later, separately

The imported tree lands under a single top-level `webapp/`, matching how `cron/`
landed. The name is deliberate: `api/` would be ambiguous in a repository whose
existing capability is already called `api-server`, and `webapp/` records where
the code came from.

Reorganizing into `internal/` + `cmd/` — the layout this repository otherwise
uses — is a later, separate rename commit that both `git blame` and `--follow`
track and that can be reviewed and reverted on its own. This is the batch
pipeline's **C5** applied unchanged: separating "move it" from "reorganize it" is
worth one interim inconsistency.

One consequence worth stating because it looks like an oversight: after the
graft, `go build ./...` produces two server binaries — `scorecard-api` from
`cmd/` and `scorecard-webapp` from `webapp/` — that answer overlapping routes.
That is the expected state until **W10**'s convergence change, not a defect.

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
directory. A `webapp/.golangci.yml` would therefore be dead file — present,
plausible-looking, and ignored. The webapp's config is excluded from the
extraction set and reconciled by hand into this repository's root config, with
its provenance recorded in `webapp/initial-graft.md`.

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

### W10 — Behavior freeze, quarantined GCP coupling, and deferred convergence

This is the decision the rest of the change rests on, and it is a harder version
of the batch pipeline's **C11** because the overlap here is genuine rather than
merely thematic. After the graft this repository will contain:

- `webapp/app/server/get_results.go` — reads `gs://ossf-scorecard-results`, falls
  back to `gs://ossf-scorecard-cron-results`, serves the webapp GET contract;
- `internal/store` + `internal/httpapi` — reads a configurable
  `gocloud.dev/blob` backend under the same key contract, serves a deliberate
  mirror of that same GET contract;
- `cron/` — writes the bucket the first one reads.

Merging them is the point of having them in one repository. Merging them *in this
change* would make the change unreviewable and, worse, unrevertible: the argument
for a staged cutover is that the imported service behaves identically to the
deployed one, and that argument evaporates the moment the code stops being the
same code.

Three concrete constraints follow, and they should be read as prohibitions:

- **No import edges** between `webapp/` and `internal/` in either direction, and
  none between `webapp/` and `cron/`. CI enforces this the same way it does for
  the pipeline.
- **The hardcoded `gs://` constants stay.** This repository's cloud-agnostic
  rules are non-negotiable *for this repository's own code*; the imported tree is
  behavior-frozen until cutover completes, exactly as `cron/`'s GCS write path
  is. A future agent reading `AGENTS.md` will find three rule violations in
  `webapp/` and want to fix them. `AGENTS.md` must say, in the imperative, that
  they are quarantined and why.
- **`openapi.yaml` is not edited.** It is simultaneously the published contract
  and the Cloud Endpoints deployment configuration, including an
  `x-google-backend` address. Changing it changes a deployed service.

**The inversion of D11.** `docs/upstream-graft.md` currently names
`ossf/scorecard-webapp` as the graft target for `internal/store` and the
`/projects` handlers, and describes this repository as an incubator whose durable
pieces travel outward. After this import that is wrong in a way an amendment
cannot fix: the destination is now in-tree. The graft does not disappear — it
becomes an in-repository convergence, which is strictly easier to land and
strictly easier to review — but the document must be rewritten to say so, and
`internal/model`'s "does not graft" rationale (**D13**, "the webapp's generated
models live upstream") needs revisiting now that those generated models are here.

The endgame is unchanged and worth restating so the deferral does not read as
abandonment: one server, serving the published contract, reading a configurable
backend, optionally scanning live, fed by a producer that writes through the same
store. This change assembles the parts in one place. A later change makes them
one thing.

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
`github.com/ossf/scorecard-webapp` stops resolving as a module. The plausible
external consumer is `app/generated/models` — the published API's request and
response types, which are exactly the sort of thing a downstream tool imports
rather than redeclares. Phase 0 verifies this against `pkg.go.dev` importers and
code search. A positive result turns removal from a delete into a deprecation
window, and is a reason the tree lands importable at `webapp/` rather than under
`internal/` (**W5**).

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
conventional — read history per-path (`git log -- webapp/`), and treat
whole-repository `git log` as a merge of three streams rather than a narrative.

It is worth noticing, though, that this is the second time; a third import should
prompt the question of whether the repository is accumulating a monorepo by
default rather than by decision.

## Relationship to the existing API server and pipeline

Stated explicitly against the areas where a reader would reasonably expect
interaction:

- **`internal/httpapi`** — unchanged. It continues to serve its mirror of the
  contract on its own binary and port. The imported server does not learn about
  it and it does not learn about the imported server.
- **`internal/store`** (**D3/D4**) — unchanged. The imported API keeps its own
  direct `gocloud.dev/blob` calls with hardcoded bucket URLs. Putting the store
  underneath it is the convergence work this move enables (**W10**), not part of
  the move.
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

- **Two servers answering one contract in one repository, both behavior-frozen
  for different reasons.** The failure mode is not a bad merge; it is a
  well-intentioned tidy-up. Mitigated by an explicit prohibition in `AGENTS.md`,
  a CI import-edge check, and this document — the same three mechanisms the
  pipeline's **C11** uses, which have held so far.
- **The website is a live client of the moved service.** Any contract drift
  during the move breaks a page nobody thought to test. The response-diff gate
  (**W11**, step 3) is the mitigation; the site's viewer path should be in the
  fixed request set.
- **Generated-code regeneration produces a large diff.** The pipeline's import
  hit this with protobuf: regenerating with a current toolchain produced 200+
  changed lines of generator modernization around a one-line semantic change.
  go-swagger will behave the same way. Decide before Phase 3 whether to pin the
  generator to the version that produced the checked-in tree (minimal diff, stale
  toolchain) or regenerate current (large diff, reproducible going forward), and
  record the choice.
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

- **External importers.** Whether anything imports
  `github.com/ossf/scorecard-webapp/app/generated/models` or any other package in
  the module. Phase 0 verifies via `pkg.go.dev` importers and code search. A
  positive result changes upstream removal from a delete to a deprecation window.
- **Where production runs after cutover.** The destination for the hosted API is
  being settled outside this change. Phases 0–5 are independent of that outcome;
  Phase 6 is sequenced against it, and Phase 7 must not begin until it is
  settled.
- **Whether the tag-triggered release convention moves.** Upstream deploys the
  production API by pushing a `v1.x` tag. This repository does not currently tag
  releases at all, and the batch pipeline's images are built by Cloud Build
  triggers rather than tags. Adopting the tag convention, replacing it, or
  running both are all viable; decide at task 4.4 rather than inheriting it by
  accident.
- **Generator pinning for regeneration.** See the risk above; decide before
  Phase 3.
- **Steering Committee artifact format.** This change is the authoritative
  artifact. Whether the committee wants a standalone summary instead is worth
  asking before the approval request goes out — the same question the pipeline's
  migration left open.
