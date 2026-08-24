# Proposal: Migrate the results API out of ossf/scorecard-webapp

## Why

`ossf/scorecard-webapp` contains two projects. One is a Nuxt website deployed to
Netlify (`scorecards-site/`, `netlify.toml`). The other is a Go REST API deployed
to Cloud Run behind Google Cloud Endpoints: the `GET` results and badge
endpoints, the signed-upload `POST` path that verifies Sigstore certificates and
Scorecard Action workflows before publishing a result, and the `openapi.yaml`
contract they are generated from. The website consumes the API over HTTPS at
runtime; neither builds against the other.

That second project belongs here, for a reason the batch pipeline's migration
did not have: **this repository already contains both halves of the data path
the API sits in the middle of.** `cron/` writes results to
`gs://ossf-scorecard-results`; the webapp API reads that exact bucket; and
`internal/store` is a provider-agnostic reimplementation of that same read path
against the same key contract. Three implementations of one pipeline currently
span two repositories, and the one in the middle is the only one still pinned to
a single cloud.

Four things make now the moment:

1. **The dependency graph is clean today.** Of 150 commits touching the API, one
   also touches the website — a 2022 project-wide copyright rename. There is no
   source-level edge in either direction: the site is JavaScript, the API is Go,
   and the site's only reference to the API is the deployed URL its viewer
   fetches. That single fact is what makes this a mechanical move.
2. **Consolidation has replaced grafting as the strategy.** Design **D11**
   (`docs/upstream-graft.md`) treats this repository as an incubator whose
   durable pieces travel outward, naming `ossf/scorecard-webapp` as the
   destination for `internal/store` and the `/projects` handlers. Two of those
   graft targets are now arriving here instead. Unifying the infrastructure in
   one repository is simpler than grafting between three, and it is what this
   change commits to (**W10**).
3. **The API is a disproportionate share of the webapp's maintenance surface.**
   88 of the 150 API commits are Dependabot; the Go module, the go-swagger
   toolchain, the Docker image, the OSS-Fuzz integration, and the CodeQL Go
   matrix all exist to serve code the website never calls. Meanwhile the site's
   own Nuxt 3 migration is in flight upstream, on a surface this move shrinks.
4. **Provider-agnosticism is this repository's stated purpose, and the API is
   the most GCP-bound component in the ecosystem.** Two hardcoded `gs://`
   constants in the read path, one in the write path, and an `x-google-backend`
   block inside the published contract itself. Nothing about moving it fixes
   that — but the exit cannot start in a repository whose reason to exist is a
   single cloud's deployment.

This is a **repository split, not a rewrite.** No API behavior changes. No
endpoint, response shape, cache header, or verification rule changes. The
website and every published badge keep working against an unchanged contract.

**Analysis basis:** `ossf/scorecard-webapp` `main` @ `39c800b`,
`ossf/scorecard-infra` `main` @ `5b90e6f`. 117 files and 150 of the webapp's 653
commits, spanning 2021-12-29 to 2026-08-07, across 12 authors.

## What Changes

- **Import the API in its entirety with full git history** (**W1**), using
  `git filter-repo` to extract and `git merge --allow-unrelated-histories` to
  graft. The merge is conflict-free by construction, because every imported path
  is renamed under a prefix this repository does not have.
- **Land everything under one top-level `api/` directory** (**W5**). Unlike
  the batch pipeline, the API is not already namespaced upstream: `main.go`,
  `openapi.yaml`, `Makefile`, `Makefile.swagger`, `COPYRIGHT.txt`, and
  `Dockerfile` live at the webapp's repository root, and three of those collide
  with files here. Renaming during the filter is mandatory, not stylistic. Any
  reorganization into this repository's `internal/` + `cmd/` layout happens after
  the graft as an ordinary rename commit.
- **Import `docs/dns.md` whole and trim it at the tip** (**W6**). It is the one
  genuinely shared document, covering DNS for both the Netlify site and the
  Cloud Run API. Importing it whole and deleting the site half here — while
  upstream deletes the API half — leaves both halves with their history.
- **Protect the imported history**: strip the webapp's 20 `v1.x` tags before
  merging (**W2**) — they are production API deploy triggers, so importing them
  is both a namespace collision and a semantic error — and rewrite `(#N)` PR
  references in 140 commit subjects to `(ossf/scorecard-webapp#N)` (**W3**).
- **Rewrite paths at the tip only** (**W4**): one commit taking
  `github.com/ossf/scorecard-webapp/app/...` to
  `github.com/ossf/scorecard-infra/api/app/...`, plus the non-Go references
  that no compiler catches — the Dockerfile's build context and `make` target,
  the Makefile's source prerequisites and output path, the `//go:generate` and
  `swagger generate --target/--spec` paths, and the `addlicense` ignore globs.
  The batch pipeline's migration discovered this class of breakage only when an
  image build failed; here it is enumerated in advance.
- **Reconcile the linter and CI by hand, not by import** (**W7**/**W8**). A
  second `.golangci.yml` under `api/` would be silently ignored by
  `golangci-lint run ./...`, so upstream's config is merged into this
  repository's root config. Upstream's `main.yml` duplicates `presubmits.yml`
  except for two things worth keeping: the `addlicense` header check and a
  GitHub token in the test environment, which the imported end-to-end specs
  require. Provenance for everything hand-ported is recorded in
  `api/initial-graft.md`, following the batch pipeline's precedent (**C13**).
- **Redeploy the imported server as-is, and take it as the preferred
  implementation** (**W10**). This is a lift-and-shift of running infrastructure,
  not a refactor. Where the imported API and `internal/httpapi` implement the
  same contract, the one already serving production wins; `internal/httpapi` and
  its supporting packages stay in place, keep building, and come off the
  deployment path. The decision is revisitable, and reversing it later costs
  nothing that this change spends.
- **Quarantine the GCP coupling rather than fixing it** (**W10**). The imported
  tree violates this repository's "no hardcoded bucket URLs" rule in three
  places and carries a Cloud Endpoints backend address inside `openapi.yaml`.
  Behavioral equivalence is the acceptance test for this change, so the coupling
  is frozen in place and explicitly out of scope, exactly as the pipeline's GCS
  write path is.
- **Retire `docs/upstream-graft.md`'s incubator framing** rather than amending
  it (**W10**). The document tells a reader that the durable pieces here graft
  outward; after this import that is wrong about two of its three targets, and a
  reader who trusts it will do the wrong work.
- **Repoint the external systems that are not in git**: the Cloud Build trigger
  and Cloud Run service that build and deploy the API, the Cloud Endpoints
  service configuration deployed from `openapi.yaml`, the domain mapping, and
  the OSS-Fuzz project (`scorecard-web`), whose build configuration in
  `google/oss-fuzz` names the source repository.
- **Inherit the release trigger, then decouple it** (**W15**). Upstream deploys
  production by pushing a `v1.x` tag into a Cloud Build trigger; that mechanism
  moves unchanged at cutover, because changing how a release fires in the same
  step that changes where the source lives gives a failure two candidate causes.
  Two things do change: releases are tagged `api/vX.Y.Z`, since this repository
  now holds more than one deployable and upstream's `v1.0.x` must keep resolving;
  and after the hold, image build and publish move to GitHub Actions producing a
  digest-addressed artifact, shrinking the provider-specific surface to a single
  deploy step. That split is the seam the eventual GCP exit needs.
- **Match the checked-in generated tree rather than regenerating current**
  (**W16**), so the import stays a pure relocation. The go-swagger version that
  produced it was never recorded and has to be recovered and pinned.
- **Cut over in stages** (**W11**): build the image here to a staging tag,
  deploy it to a non-production Cloud Run revision, compare responses against
  production for a fixed request set, then repoint. Nothing is deleted upstream
  until production has served traffic from an infra-built image for a full hold
  period, so rollback stays a redeploy of the previous revision.
- **Remove the moved code from `ossf/scorecard-webapp` last** (**W12**), leaving
  a site-only repository: a rewritten `README.md`, a redirect stub, the API half
  of `docs/dns.md` removed, and `go.mod`/`go.sum` deleted — which retires the
  `github.com/ossf/scorecard-webapp` module and is why Phase 0 verifies external
  importers before, not after.

## Capabilities

### New Capabilities

- `hosted-api`: the results API now hosted here — the `GET` results and badge
  endpoints, the signed-upload `POST` path and its Sigstore/Rekor and workflow
  verification rules, the object-store read and write paths, CDN purge, and the
  cache-control semantics every downstream consumer depends on.
- `api-contract`: `openapi.yaml` as an externally published, externally
  consumed contract, the go-swagger generation workflow that derives the server
  and client from it, and the requirement that regeneration be reproducible.
- `api-deployment`: the deployment surface — the container image, the Cloud Run
  service and Cloud Endpoints configuration, the tag-triggered release path, the
  fuzzing integration, and the staged cutover and rollback contract.
- `repository-history`: the verifiable history-preservation guarantees of this
  import. The capability is shared with the batch pipeline's migration; this
  change adds requirements scoped to the API import.

### Modified Capabilities

<!--
None. api-server, result-store, result-cache, live-scan, feature-flags, and
upstream-fallback are untouched: the imported API lands beside them, not through
them. Reconciling the two servers is the strategically interesting follow-on and
is deliberately deferred (design W10) — as is the equivalent question for the
batch pipeline (C11).
-->

## Impact

- **New code:** 117 files under `api/` — 20 hand-written server files plus 39
  test fixtures, 32 go-swagger generated files plus 19 embedded Swagger UI
  assets, and 7 root-level files (entrypoint, contract, build wiring, Dockerfile,
  copyright header template, shared DNS doc). No existing package under
  `internal/`, `cmd/`, or `cron/` is modified.
- **Repository history:** ~150 additional commits dating to 2021-12-29, grafted
  via a second unrelated-histories merge. This repository will then carry three
  independent roots.
- **Dependencies:** adds the go-openapi/go-swagger runtime stack,
  `google/go-github`, `rhysd/actionlint`, `transparency-dev/merkle`,
  `cyberphone/json-canonicalization`, `rs/cors`, and the Ginkgo/Gomega test
  framework. `gocloud.dev` is already a direct dependency at the same version.
- **Build and CI:** one new image build, an `addlicense` header check, a
  token-bearing test environment, and a go-swagger regeneration check. Longer CI,
  though far less than the pipeline's six images added.
- **Tests reach the network.** The imported end-to-end specs verify real Sigstore
  certificates against live Rekor and resolve commits against the live GitHub
  API. They skip cleanly when Rekor's search index is unavailable but need a
  GitHub token to pass. This repository's test job currently supplies neither.
- **External systems:** the Cloud Build trigger, Cloud Run service, Cloud
  Endpoints service configuration, domain mapping, and the `scorecard-web`
  OSS-Fuzz project all reference `ossf/scorecard-webapp` and are configured
  outside git. None is revertible by `git revert`.
- **Upstream (`ossf/scorecard-webapp`):** becomes a website-only repository. It
  loses the API tree, the Go module, the Go CI, the Dockerfile, the fuzzing
  integration, and the API half of its DNS documentation. Its release tags stop
  meaning "deploy the API".
- **Compatibility:** deleting `go.mod` upstream stops
  `github.com/ossf/scorecard-webapp` resolving at `@latest`. One external
  consumer exists — `jetstack/tally` imports `app/generated/models` — and it
  survives, because it pins the released `v1.0.5` and the module proxy holds
  published versions immutably. The obligation this creates is a prohibition:
  upstream's release tags must not be deleted (**W12**). No deprecation window
  is needed.
- **Governance:** requires Scorecard Steering Committee sign-off, admin sign-off
  on the GCP project hosting the API, and review from at least one non-Steering
  maintainer. The community is notified before cutover and again before upstream
  removal.

## Non-goals

- **Any change to API behavior**, response shape, status codes, cache headers,
  or verification rules. Behavioral equivalence is the acceptance test.
- **Any change to the published `openapi.yaml` contract**, including its
  `x-google-backend` block. Reworking the contract for a non-GCP deployment is
  the follow-on the move enables, not part of the move.
- **Removing the hardcoded `gs://` constants** or putting `gocloud.dev/blob`
  backend selection underneath the API. Same reason (**W10**).
- **Reconciling the imported API with `internal/httpapi`, `internal/store`, or
  `internal/orchestrator`.** Deferred to its own change (**W10**); this one must
  stay behavior-preserving to be revertible. Preferring the imported
  implementation settles which side of that reconciliation is the starting
  point — it does not perform it here.
- **Deleting `internal/httpapi` or its supporting packages.** Coming off the
  deployment path is not removal, and the choice is meant to stay cheap to
  revisit (**W10**).
- **Restructuring `api/` into this repository's `internal/` + `cmd/`
  layout** (**W5**).
- **Moving the website.** `scorecards-site/`, `netlify.toml`, and the site's CI
  stay in `ossf/scorecard-webapp`.
- **Choosing where the production API ultimately runs.** That is being settled
  outside this change; the cutover phase is sequenced against it rather than
  presuming it.
- **The message broker, warm-cache scheduler, and analytics index** remain
  deferred to later changes.
