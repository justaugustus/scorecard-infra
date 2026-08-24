# Tasks: Migrate the results API out of ossf/scorecard-webapp

Groups map to the migration's phases. Each group is independently reviewable and
independently revertible. **Group 7 MUST NOT begin until group 6 has held clean
for the agreed period** (**W11**).

Counts below are gates, not estimates: they come from the analysis basis
(`ossf/scorecard-webapp` `main` @ `39c800b`) and a deviation means the filter
selected the wrong thing.

## 0. Pre-work / decisions

- [x] 0.1 **One external importer found, and it does not block a clean delete.**
      `jetstack/tally` imports `app/generated/models` from four of its packages —
      the predicted consumer (**W12**). Nothing imports the generated client, the
      `restapi` package, or the module root. But tally depends on
      `github.com/ossf/scorecard-webapp v1.0.5`, a *released version*, and the
      module proxy holds all 20 published versions immutably
      (`v1.0.5` resolves with its commit hash, independent of the repository's
      state). Deleting `go.mod` from upstream `main` therefore breaks
      `@latest`/`@main` resolution and new consumers, not pinned builds. tally is
      also dormant — last push 2023-11-13, still on `scorecard/v4`.
      **Conclusion: clean delete is safe, with one added obligation — upstream's
      release tags must not be deleted** (task 7.9). No deprecation window.
- [ ] 0.2 Confirm `git-filter-repo` is installed and that `re` is available in
      the `--message-callback` context without an explicit import (**W1**).
- [ ] 0.3 Dry-run the filter and confirm the prefix-matching behavior of
      `--path Makefile` / `--path-rename Makefile:api/Makefile` selects and
      relocates `Makefile.swagger` as intended, and selects nothing else (**W1**).
- [x] 0.4 **Decided: match the checked-in tree.** Regeneration must be a no-op,
      so the Phase 3 diff stays empty and the import stays a pure relocation.
      Toolchain currency is a separate concern from a migration whose acceptance
      test is that nothing changed.
- [ ] 0.4a **Recover the generator version, because nobody wrote it down.** The
      generated files carry no version stamp and the webapp pins go-swagger
      nowhere — its `Makefile` invokes whatever `swagger` is on `PATH`, the same
      unpinned-tool problem the pipeline import hit with `protoc`. Bisect
      go-swagger releases current around 2026-02-05 (`395f1f1`, the last commit
      to regenerate the tree) until `make swagger` produces no diff, then pin
      that version where CI enforces it. If no release reproduces the tree
      exactly, say so and escalate to 0.4 rather than accepting a small diff
      quietly — a tree nobody can regenerate is a tree nobody can safely change.
- [x] 0.5 **Decided: administrative override.** 37 imported commits carry no
      sign-off and no honest code fix exists (**W13**). Same resolution the
      pipeline import reached, agreed in advance this time rather than at merge.
      Record who applied it and on which pull request.
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
- [ ] 1.3 Run `git filter-repo` over the API path set with the `api/` renames
      and the `(#N)` → `(ossf/scorecard-webapp#N)` rewrite (**W1**/**W3**).
      `docs/dns.md` is retained unrenamed (**W6**).
- [ ] 1.4 Gate: **150 commits**, **0 merge commits** — the webapp squash-merges,
      so any merge commit in the result means the filter kept something
      unexpected.
- [ ] 1.5 Gate: **117 files**, all under `api/` except `docs/dns.md`.
- [ ] 1.6 Gate: **0 tags**.
- [ ] 1.7 Gate: `git log --follow -- api/app/server/get_results.go` resolves
      through the 2022 go-swagger restructure into its pre-rename
      `app/get_results.go` history, and
      `api/app/server/post_results_test.go` follows back through
      `app/signing_test.go`.
- [ ] 1.8 Gate: `git blame` on `api/app/server/post_results.go` attributes to
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
      `api/` or `docs/dns.md`.
- [ ] 2.2 Verify rename tracking and blame survive the merge (re-run 1.7/1.8
      against the merged branch).
- [ ] 2.3 Verify zero tags arrived with the merge.
- [ ] 2.4 Remove the extraction remote. The branch stays **local and unpushed**
      until the extraction has been reviewed.
- [ ] 2.5 Record what could not be imported and why, in
      `api/initial-graft.md`: the linter config (**W7**), the CI fragments
      (**W8**), and the originating commits for each. Follows the batch
      pipeline's `cron/initial-graft.md` precedent (**C13**) — the note lives
      inside the imported tree, next to the code it explains.

## 3. Make it build

- [ ] 3.1 Rewrite import paths at the tip: `github.com/ossf/scorecard-webapp/app/`
      → `github.com/ossf/scorecard-infra/api/app/` (**W4**).
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
      generated tree must be scoped to `api/app/generated/`; no linter is
      disabled repository-wide to accommodate the import.
- [ ] 3.6 Verify no import edges: `api/` imports nothing from `internal/`,
      `cmd/`, or `cron/`, and none of them imports `api/` (**W10**).
- [ ] 3.7 Regenerate from `openapi.yaml` with the generator chosen in 0.4 and
      confirm the tree is reproducible — `make swagger` followed by
      `git diff --exit-code` is clean, and `configure_scorecard.go` is unchanged.
- [ ] 3.8 `go build ./...`, `go test ./... -race`, and `golangci-lint run ./...`
      pass with all three trees included. Record which imported tests reach the
      network and what they need (**W9**).
- [ ] 3.9 Verify the imported binary still starts and serves: run it locally
      against a `fileblob`- or `memblob`-backed fixture where possible, and
      record which endpoints cannot be exercised without GCS credentials.
- [ ] 3.10 Confirm the imported binary is still named `scorecard-webapp` and the
      incubator's is still `scorecard-api` (**W5**). The names invert reality and
      renaming is tempting while the Dockerfile is open — but it changes image
      contents, and image equivalence is the cutover gate. Rename after 6.7.

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
- [ ] 4.7 Add the import-edge CI check for `api/` (**W10**), matching the one
      that guards `cron/`.
- [ ] 4.8 Measure the presubmit wall-clock and total-compute change, as was done
      for the pipeline import. Report the numbers rather than an impression.
- [ ] 4.9 Adopt the release-trigger sequence in **W15**: inherit upstream's
      tag-triggered build and deploy unchanged at cutover, and tag
      `api/vX.Y.Z` rather than `vX.Y.Z`. The namespace is the part that must be
      right before the *first* tag — renaming a release namespace later is worse
      than choosing it awkwardly now.
- [ ] 4.10 File the follow-up for **W15** step 3 — move image build and publish
      into GitHub Actions producing a digest-addressed image, leaving the
      provider-specific step as "deploy this digest". Not in this change; it is
      the seam the GCP exit needs, and it should exist as a tracked item rather
      than as a paragraph in a design document.

## 5. Documentation and repository identity

- [ ] 5.1 Rewrite `docs/upstream-graft.md` around consolidation rather than
      amending its incubator framing (**W10**). Two of **D11**'s three graft
      targets are now in-tree, so the document's central claim — that the durable
      pieces here travel outward — is wrong, and a reader who trusts it will do
      the wrong work. State what is genuinely still graftable, and record the
      rationales that stopped holding rather than deleting them silently.
- [ ] 5.2 Update `AGENTS.md`: four parts rather than three, an `api/` component
      map, the behavior-freeze rule, an **imperative** statement that the
      hardcoded `gs://` constants and `openapi.yaml`'s `x-google-backend` block
      are quarantined and must not be "fixed", and which of the two servers is on
      the deployment path (**W10**).
- [ ] 5.3 Update `openspec/config.yaml` context: the imported API, the
      two-servers-one-contract state, which one ships, and the deferred
      convergence.
- [ ] 5.4 Add a `README.md` section for the imported API: endpoints, the
      contract's role, the image, the deployment surface, and where the website
      that consumes it lives.
- [ ] 5.5 Record the deprioritization where it will be read, not only here: a
      note in `internal/httpapi` stating that it is off the deployment path, why,
      and that the decision is revisitable (**W10**). A package nobody deploys and
      nobody has told is how stale code gets written against.
- [ ] 5.6 Note in `internal/model`'s design record that **D13**'s rationale
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
- [ ] 7.9 **Do not delete `ossf/scorecard-webapp`'s release tags.** Deleting the
      code is safe; deleting `v1.0.5` is what breaks `jetstack/tally`, the one
      external consumer (0.1, **W12**). This change strips those same tags from
      the *import* (**W2**) — the operations look alike and have opposite
      consequences, so state the distinction in the removal pull request rather
      than trusting it to be obvious.
- [ ] 7.10 Verify the obligation holds rather than assuming it: after removal,
      confirm `go mod download github.com/ossf/scorecard-webapp@v1.0.5` still
      resolves.

## 8. Change closeout

- [ ] 8.1 `openspec validate migrate-api --strict` passes.
- [ ] 8.2 Verify every success criterion in the proposal and design is met.
- [ ] 8.3 Archive the change once implemented and merged.
