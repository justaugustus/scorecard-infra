# Tasks: Migrate the results API out of ossf/scorecard-webapp

Groups map to the migration's phases. Each group is independently reviewable and
independently revertible. **Group 7 MUST NOT begin until group 6 has held clean
for the agreed period** (**W11**).

Counts below are gates, not estimates: they come from the analysis basis
(`ossf/scorecard-webapp` `main` @ `39c800b`) and a deviation means the filter
selected the wrong thing.

## 0. Pre-work / decisions

- [ ] 0.1 Verify no external Go importers of `github.com/ossf/scorecard-webapp`,
      checking `app/generated/models` specifically — the published request and
      response types are the plausible consumer (**W12**). Use `pkg.go.dev`
      importers plus code search. A positive result turns Phase 7's deletion into
      a deprecation window and must be resolved before approval, not after.
- [ ] 0.2 Confirm `git-filter-repo` is installed and that `re` is available in
      the `--message-callback` context without an explicit import (**W1**).
- [ ] 0.3 Dry-run the filter and confirm the prefix-matching behavior of
      `--path Makefile` / `--path-rename Makefile:webapp/Makefile` selects and
      relocates `Makefile.swagger` as intended, and selects nothing else (**W1**).
- [ ] 0.4 Decide generator pinning for go-swagger regeneration: pin to the
      version that produced the checked-in tree, or regenerate current. Record
      the choice and its reason; it determines the size of the Phase 3 diff.
- [ ] 0.5 Decide the DCO resolution — administrative override or scoped check —
      **before** review opens, and record who agreed it (**W13**).
- [ ] 0.6 Confirm ownership and admin access for every external system to be
      repointed: Cloud Build trigger, Cloud Run service, Cloud Endpoints service
      configuration, domain mapping, Fastly, and the `scorecard-web` OSS-Fuzz
      project (**W8**/**W11**).
- [ ] 0.7 Confirm the production hosting destination is settled, or explicitly
      agree that phases 0–5 proceed without it and that phase 6 waits.
- [ ] 0.8 Obtain approvals: Scorecard Steering Committee, GCP project admin, and
      at least one non-Steering maintainer. Ask the committee whether it wants a
      standalone summary rather than this OpenSpec change as the artifact.

## 1. History extraction, reviewed in isolation

**Nothing is pushed to `ossf/scorecard-infra` in this group.**

- [ ] 1.1 Clone `ossf/scorecard-webapp` fresh and single-branch to
      `../scorecard-api-extract`; record the tip SHA and total commit count and
      confirm they match the analysis basis (**W1**).
- [ ] 1.2 Delete all 20 tags in the clone before filtering (**W2**).
- [ ] 1.3 Run `git filter-repo` over the API path set with the `webapp/` renames
      and the `(#N)` → `(ossf/scorecard-webapp#N)` rewrite (**W1**/**W3**).
      `docs/dns.md` is retained unrenamed (**W6**).
- [ ] 1.4 Gate: **150 commits**, **0 merge commits** — the webapp squash-merges,
      so any merge commit in the result means the filter kept something
      unexpected.
- [ ] 1.5 Gate: **117 files**, all under `webapp/` except `docs/dns.md`.
- [ ] 1.6 Gate: **0 tags**.
- [ ] 1.7 Gate: `git log --follow -- webapp/app/server/get_results.go` resolves
      through the 2022 go-swagger restructure into its pre-rename
      `app/get_results.go` history, and
      `webapp/app/server/post_results_test.go` follows back through
      `app/signing_test.go`.
- [ ] 1.8 Gate: `git blame` on `webapp/app/server/post_results.go` attributes to
      original authors, not to the filter.
- [ ] 1.9 Gate: **0 bare `(#N)` references remain**; 140 commits carry rewritten
      `(ossf/scorecard-webapp#N)` references. History spans 2021-12-29 to
      2026-08-07 across 12 authors.
- [ ] 1.10 Confirm the one commit that touches both halves
      (`b7e0f8d`, "Change project name to OpenSSF Scorecard on website and in
      copyright") survives with only its API-side changes and is not empty.

## 2. Graft into scorecard-infra

- [ ] 2.1 Create a branch off `main`; merge the filtered history with
      `--allow-unrelated-histories`. Expect conflict-free — nothing here occupies
      `webapp/` or `docs/dns.md`.
- [ ] 2.2 Verify rename tracking and blame survive the merge (re-run 1.7/1.8
      against the merged branch).
- [ ] 2.3 Verify zero tags arrived with the merge.
- [ ] 2.4 Remove the extraction remote. The branch stays **local and unpushed**
      until the extraction has been reviewed.
- [ ] 2.5 Record what could not be imported and why, in
      `webapp/initial-graft.md`: the linter config (**W7**), the CI fragments
      (**W8**), and the originating commits for each. Follows the batch
      pipeline's `cron/initial-graft.md` precedent (**C13**) — the note lives
      inside the imported tree, next to the code it explains.

## 3. Make it build

- [ ] 3.1 Rewrite import paths at the tip: `github.com/ossf/scorecard-webapp/app/`
      → `github.com/ossf/scorecard-infra/webapp/app/` (**W4**).
- [ ] 3.2 Fix every non-Go reference in the **W4** table — Dockerfile build
      context and make target, Makefile prerequisites and output path, swagger
      generate paths, the `SWAGGER_GEN` find expression, the `//go:generate`
      directive, and the `addlicense` ignore globs. Grep for
      `scorecard-webapp` across the imported tree afterwards; the only surviving
      hits should be in files that *describe* the migration.
- [ ] 3.3 Trim `docs/dns.md` to the API half and link the site half's new
      location upstream (**W6**).
- [ ] 3.4 Reconcile `go.mod` / `go.sum`. Confirm `gocloud.dev` resolves to a
      single version across the pipeline, the API server, and the import.
- [ ] 3.5 Reconcile `.golangci.yml` by hand (**W7**). Exclusions for the
      generated tree must be scoped to `webapp/app/generated/`; no linter is
      disabled repository-wide to accommodate the import.
- [ ] 3.6 Verify no import edges: `webapp/` imports nothing from `internal/`,
      `cmd/`, or `cron/`, and none of them imports `webapp/` (**W10**).
- [ ] 3.7 Regenerate from `openapi.yaml` with the generator chosen in 0.4 and
      confirm the tree is reproducible — `make swagger` followed by
      `git diff --exit-code` is clean, and `configure_scorecard.go` is unchanged.
- [ ] 3.8 `go build ./...`, `go test ./... -race`, and `golangci-lint run ./...`
      pass with all three trees included. Record which imported tests reach the
      network and what they need (**W9**).
- [ ] 3.9 Verify the imported binary still starts and serves: run it locally
      against a `fileblob`- or `memblob`-backed fixture where possible, and
      record which endpoints cannot be exercised without GCS credentials.

## 4. CI and image builds, in parallel with production

**Production still runs images built from `ossf/scorecard-webapp` throughout.**

- [ ] 4.1 Add the API image target to `build-images.yml` alongside the pipeline's
      six. Verify in CI, not locally — the batch pipeline's equivalent defect was
      a stale path inside a Dockerfile that only an image build could catch
      (**W4**).
- [ ] 4.2 Add the `addlicense` header check to `presubmits.yml` with the ignore
      globs corrected for the new paths (**W8**).
- [ ] 4.3 Supply a GitHub token to the test job so the imported end-to-end specs
      run rather than fail or silently skip (**W9**).
- [ ] 4.4 Add the go-swagger regeneration check as a blocking presubmit, asserting
      both that generated output matches `openapi.yaml` and that
      `configure_scorecard.go` is untouched (**W8**).
- [ ] 4.5 Port `cifuzz.yml` (**W8**). The OSS-Fuzz project itself is repointed at
      cutover (6.4), not here — porting the workflow without repointing the
      project leaves fuzzing running against the old source.
- [ ] 4.6 Add the Dockerfile directory to `.github/dependabot.yml`, and confirm
      the gomod group picks up the newly direct dependencies.
- [ ] 4.7 Add the import-edge CI check for `webapp/` (**W10**), matching the one
      that guards `cron/`.
- [ ] 4.8 Measure the presubmit wall-clock and total-compute change, as was done
      for the pipeline import. Report the numbers rather than an impression.
- [ ] 4.9 Decide and record the release-trigger convention (open question): keep
      upstream's tag-triggered deploy, replace it, or run both.

## 5. Documentation and repository identity

- [ ] 5.1 Rewrite `docs/upstream-graft.md` rather than amending it (**W10**). The
      graft target for `internal/store` and the `/projects` handlers is now
      in-tree; the document currently says otherwise, and a reader who trusts it
      will do the wrong thing.
- [ ] 5.2 Update `AGENTS.md`: four parts rather than three, a `webapp/` component
      map, the behavior-freeze rule, and an **imperative** statement that the
      hardcoded `gs://` constants and `openapi.yaml`'s `x-google-backend` block
      are quarantined and must not be "fixed" (**W10**).
- [ ] 5.3 Update `openspec/config.yaml` context to describe the imported API,
      the two-servers-one-contract state, and the deferred convergence.
- [ ] 5.4 Add a `README.md` section for the imported API: endpoints, the
      contract's role, the image, the deployment surface, and where the website
      that consumes it lives.
- [ ] 5.5 Note in `internal/model`'s design record that **D13**'s rationale
      changed — the webapp's generated models are now in-tree (**W10**).

## 6. Production cutover

- [ ] 6.0 Capture the before-state of every external system: Cloud Build trigger
      configuration, Cloud Run service and current revision, Cloud Endpoints
      service configuration, domain mapping, Fastly configuration, and the
      OSS-Fuzz project's build settings (**W11**).
- [ ] 6.1 Build and publish the API image to a **staging tag**, never the tag the
      production service consumes.
- [ ] 6.2 Deploy a non-production Cloud Run revision with no traffic assigned.
- [ ] 6.3 Run the response diff against production (**W11**, step 3): known-good
      repository, repository with no results, pinned commit, malformed request,
      each badge style, and the request the website's viewer makes. Compare
      status codes, bodies, and `Cache-Control` / `Surrogate-Control` headers.
- [ ] 6.4 Repoint the Cloud Build trigger, shift traffic, and repoint the
      `scorecard-web` OSS-Fuzz project.
- [ ] 6.5 Notify the Scorecard community before this cutover.
- [ ] 6.6 Confirm a Scorecard Action upload completes end to end through the
      `POST` path — a broken publish path is silent, unlike a broken read.
- [ ] 6.7 Hold for the agreed period, watching error rates and the badge path.
      **Rollback:** shift traffic back to the captured prior revision; the code
      still exists upstream, so no code restoration is needed.

## 7. Removal from ossf/scorecard-webapp

- [ ] 7.1 Notify the Scorecard community again before removal.
- [ ] 7.2 Delete `app/`, `main.go`, `openapi.yaml`, `Makefile`,
      `Makefile.swagger`, `COPYRIGHT.txt`, `Dockerfile`, `go.mod`, and `go.sum`.
- [ ] 7.3 Remove the Go CI: `main.yml`, `cifuzz.yml`, the Go half of the CodeQL
      matrix, and the gomod and docker Dependabot entries.
- [ ] 7.4 Trim `docs/dns.md` to the site half (**W6**).
- [ ] 7.5 Rewrite `README.md` to describe a website-only repository, with a
      pointer to the API's new home.
- [ ] 7.6 Update `.github/CODEOWNERS` and any issue or PR templates that route
      API reports to the wrong repository.
- [ ] 7.7 Confirm the site still builds and deploys, and that its viewer still
      resolves against the API.
- [ ] 7.8 Confirm nothing in the remaining tree references the removed paths.

## 8. Change closeout

- [ ] 8.1 `openspec validate migrate-api --strict` passes.
- [ ] 8.2 Verify every success criterion in the proposal and design is met.
- [ ] 8.3 Archive the change once implemented and merged.
