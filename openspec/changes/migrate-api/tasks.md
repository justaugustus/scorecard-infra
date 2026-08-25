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

- [x] 1.1 Cloned `ossf/scorecard-webapp` fresh and single-branch to
      `../scorecard-api-extract`; tip `39c800b`, 653 commits, 20 tags — matching
      the analysis basis exactly (**W1**).
- [x] 1.2 Deleted all 20 tags in the clone before filtering (**W2**).
- [x] 1.3 Ran `git filter-repo` over the API path set with the `api/` renames
      and the `(#N)` → `(ossf/scorecard-webapp#N)` rewrite (**W1**/**W3**).
      `docs/dns.md` retained unrenamed (**W6**). The `--dry-run` confirmed `re`
      is available in the `--message-callback` context without an explicit
      import (0.2). **Took two attempts:** the first omitted `Makefile.swagger`
      on the mistaken theory that `--path Makefile` selects it by string prefix.
      It does not — see the corrected caution in **W1**.
- [x] 1.4 Gate passed — **150 commits, 0 merge commits**, exactly as
      predicted.
- [x] 1.5 Gate passed on the second run — **117 files**, 116 under `api/` plus
      `docs/dns.md`, diffed file-by-file against the expected list rather than
      counted. The first run's 116 is what surfaced the `Makefile.swagger`
      defect; a count-only check would have found it, but the diff is what named
      it.
- [x] 1.6 Gate passed — **0 tags**.
- [x] 1.7 Gate passed — `api/app/server/get_results.go` follows 9 commits back
      through the 2022 go-swagger restructure into `app/get_results.go`.
      `api/app/server/post_results_test.go` follows 14 commits back through
      **two** renames: `app/signing_test.go` → `app/post_results_test.go` →
      `app/server/post_results_test.go`.
- [x] 1.8 Gate passed — blame on `api/app/server/post_results.go` attributes to
      Azeem Shaikh (404 lines), Spencer Schrock (165), Piotr P. Karwasz, Naveen,
      asraa and others; nothing to the filter.
- [x] 1.9 Gate passed — **0 bare `(#N)` references remain**; 140 commits carry
      rewritten `(ossf/scorecard-webapp#N)` references. History spans 2021-12-29
      to 2026-08-07 across 12 authors.
- [x] 1.10 Confirmed — the both-halves commit (`b7e0f8d` upstream, `2474424`
      after filtering) survives with 47 API-side files changed and no site
      content. Not empty.

## 2. Graft into scorecard-infra

- [x] 2.1 Branch `import-api` created off `main` at `5b90e6f`; filtered history
      merged with `--allow-unrelated-histories`, conflict-free as predicted (117
      files staged, no conflicts). Repo now at 690 commits and **three roots**
      (**W14**).
- [x] 2.2 Verified — rename tracking and blame survive the merge. Re-running
      1.7 against the merged branch still resolves `post_results_test.go` 14
      commits back through both renames, and `get_results.go` 9 commits back.
- [x] 2.3 Verified — zero tags arrived with the merge.
- [x] 2.4 Extraction remote removed. Branch is **local and unpushed** — the
      filtered result at `../scorecard-api-extract` and this branch are both
      reviewable before anything reaches the remote.
- [x] 2.5 `api/initial-graft.md` written, following `cron/initial-graft.md`.
      Records the import's terms and stats, the per-file reason each unimported
      thing stayed behind — which differs from the pipeline's case, where the
      obstacle was uniformly "shared with the engine" — and the originating
      commits. Notes that `main.yml` and `.golangci.yml` landed upstream on the
      same day in June 2022, six months after the repository was created: the API
      ran without CI or a linter for its first several months, which is worth
      knowing before reading an early commit's style as deliberate.

## 3. Make it build

- [x] 3.1 Import paths rewritten in 18 Go files. Every occurrence was a module
      prefix; nothing needed hand-inspection (**W4**).
- [x] 3.2 Non-Go references fixed — **and the W4 table over-predicted.** Because
      the whole tree moved together and its recipes are relative to it, the
      Makefile prerequisites, the swagger generate paths, the `SWAGGER_GEN` find
      expression and the `//go:generate` directive are all still correct
      unchanged. Only two things genuinely broke, both because the module root is
      now two levels up: the Dockerfile (`make -C api`, and the binary copied
      from `src/api/`) and the Makefile's docker target (build context `..`).
      Upstream's `linter` target was dropped — it invoked a `.golangci.yml` that
      **W7** deliberately did not import, so it referenced a file that does not
      exist. Root Makefile gains `api-build` / `api-swagger` / `api-docker`
      delegating targets. Surviving `scorecard-webapp` strings are the binary
      name (**W5**) and upstream test-fixture repository names, both correct.
- [x] 3.3 `docs/dns.md` trimmed to the API half, with a note recording that it
      arrived whole and that the website half stays upstream (**W6**).
- [x] 3.4 `go.mod` seeded with the webapp's 21 direct requirements **at their
      pinned versions** before tidying, so the imported code builds against what
      production runs rather than whatever resolved latest. **20 of 21 held
      exactly.** The one that moved — `AdaLogics/go-fuzz-headers` — moved because
      this module already required a newer revision; MVS max, unavoidable.
      `gocloud.dev` was already at v0.46.0 here, matching the webapp exactly.
      MVS also nudged some pre-existing indirect deps (`actionlint` v1.7.9 →
      v1.7.12, `x/net` v0.56.0 → v0.58.0, `x/mod` v0.37.0 → v0.39.0 and similar).
      `actionlint` is the one worth noting: the Scorecard engine links it, and
      `cron/` is behavior-frozen — the bump is a consequence of two projects
      sharing one module, not a choice.
- [x] 3.5 `.golangci.yml` reconciled (**W7**). 41 issues, **all** explained by
      config lineage rather than by defects: the webapp ran `default: none` with
      an explicit enable list, this config descends from `ossf/scorecard`'s and
      enables strictly more. All of it was in hand-written source, so the
      generated-path exclusion the task anticipated would not have reached any of
      it. Resolved by applying the imported tree's own historical linter set to
      `api/` only, with the block marked for removal when the freeze lifts (5.7).
      No linter is disabled repository-wide.
      **`govet` was deliberately left enabled**, and it earned it — see 3.8.
- [x] 3.6 Verified clean both directions — `api/` imports nothing from
      `internal/`, `cmd/`, or `cron/`, and none of them imports `api/` (**W10**).
- [ ] 3.7 Blocked on 0.4a, which trails past this week: the generator version
      that produced the checked-in tree is unrecorded, so there is nothing to
      regenerate *with* yet. `make api-swagger` exists and is wired.
- [x] 3.8 `go build ./...` and `golangci-lint run ./...` are clean with all
      three trees included. `go test ./... -race` is clean **except** for the
      imported end-to-end specs, which reach the network — see 3.8a and 3.8b.
      Leaving `govet` enabled for `api/` caught a real defect that would
      otherwise have shipped: a non-constant format string in `api/main.go`.
      It is not a lint opinion — `go test` runs the vet printf check, so the
      package **failed to build under test**. Upstream never saw it because its
      CI only ever ran `cd app/server && go test`, never the root package.
      Fixed to `fmt.Fprint`, matching the adjacent line and provably identical
      output for the current string. First deliberate deviation from the
      behavior freeze, and a narrow one.
- [x] 3.8a **Resolved: the GitHub-API e2e specs need a token, and with one they
      pass.** `githubVerifier_contains` calls `api.github.com`; unauthenticated it
      hits the 60/hour per-IP limit and was red one local run in three. Fixed by
      supplying `github.token` in CI (4.3). **Verified in CI (PR #49): all specs
      pass.** This is what **W9** predicted, and the fix was configuration.
- [x] 3.8b **Corrected. The Sigstore verification specs do run, and they pass.**
      An earlier note here claimed all five skipped unconditionally because Rekor
      v2 removed the search-by-hash index, and concluded the publish path's
      certificate verification had no working end-to-end coverage anywhere. That
      was wrong, and it was wrong in the direction that matters — it understated
      existing coverage and would have justified skipping verification work.
      **CI reports 8 passed, 0 skipped** (PR #49), at 64.6% statement coverage
      for `api/app/server`.
      The local result — 3 passed, 5 skipped — was an artifact of this authoring
      environment: it receives an HTML `403` from *every* Rekor endpoint,
      including `/api/v1/log`, so the block is a network proxy rather than a
      removed API. The upstream skip guard is real and its comment does cite
      Rekor v2, so the coverage may still be intermittent for other consumers —
      but it is present and green today.
      Task 6.6 (confirm a real Action upload end to end) is therefore ordinary
      cutover verification again, not the sole check standing in for absent
      coverage.
- [x] 3.9 Binary builds via `make api-build`, starts, and serves. The badge
      path returns `302` to `img.shields.io` correctly — it is a pure redirect
      with no storage access, contrary to the AWS memo's description of it as
      generating badges. `/projects/{host}/{org}/{repo}` returns `404` without
      GCS credentials, which is the read path failing closed rather than
      erroring; it cannot be exercised further locally.
- [x] 3.10 Confirmed — the imported binary is still named `scorecard-webapp` and the
      incubator's is still `scorecard-api` (**W5**). The names invert reality and
      renaming is tempting while the Dockerfile is open — but it changes image
      contents, and image equivalence is the cutover gate. Rename after 6.7.

## 4. CI and image builds, in parallel with production

**Production still runs images built from `ossf/scorecard-webapp` throughout.**

- [x] 4.1 `api-docker` added to `build-images.yml`'s matrix. **Verified in CI
      (PR #49): the image builds, in 2m0s.** It could not be built locally — the
      Docker daemon here cannot reach `registry-1.docker.io` (TLS handshake
      timeout on a direct pull, nothing cached), the same interception the
      pipeline import hit. So the Dockerfile path fixes from 3.2 were unproven
      until this run, which is exactly the defect class that only an image build
      catches.
- [x] 4.2 `license-headers` job added, using `-check` (report, don't rewrite)
      rather than upstream's rewrite-then-`git diff` pair. **Scoped to `api/`:**
      run repository-wide it flags four pre-existing files —
      `.github/dependabot.yml` and three workflows — that have no header and
      nothing to do with this import. Widening it is a separate change.
- [x] 4.3 `GITHUB_AUTH_TOKEN: ${{ github.token }}` added to the test job, fixing
      the rate-limit flake in 3.8a. **Verified in CI (PR #49): 8 specs pass, 0
      skip**, `api/app/server` at 64.6% statement coverage.
- [ ] 4.4 Blocked on 0.4a, same as 3.7: there is no pinned generator to check
      against yet.
- [ ] 4.5 Deferred with the OSS-Fuzz repointing, per the scope call for this
      week. Porting the workflow alone would leave fuzzing pointed at the old
      source anyway.
- [x] 4.6 `/api` added to `.github/dependabot.yml` as its own docker entry.
      Kept separate from the pipeline glob and the API server's root entry for
      the reason those two are already separate: it is a third `golang` tag
      lineage, and a shared group keyed on dependency name would collapse
      unrelated bumps into one pull request.
- [x] 4.7 `import-edges` job added — **there was no existing check to match.**
      The task assumed one guarded `cron/`; what exists is a checkbox in the
      pull-request template, which is a promise rather than a guard. The job
      covers both frozen trees and both directions, since excluding `cron/`
      from the same three-line script would have been artificial. Verified twice:
      it passes on the current tree, and it exits 1 on a planted violation. A
      guard that has never been seen to fail is not yet a guard.
- [x] 4.8 **Measured from PR #49.** Presubmits wall clock is ~4m15s, set by the
      `test` job — which is also where the API landed, since its end-to-end specs
      are network-bound. The two new jobs are not on the critical path
      (`license-headers` 35s, `import-edges` 17s) and `build`/`lint` finish at
      3m33s/3m50s. Image builds are a separate ~3m51s path, set by
      `cron-controller-docker`; `api-docker` is among the faster targets at 2m0s.
      Net effect of adding the API: no change to the critical path's shape, and
      one more image build running in parallel with six others.
- [x] 4.9 Release trigger is `api/vX.Y.Z`, implemented in
      `publish-api-image.yml` and documented in its header with the reasoning —
      two deployables here, and upstream's `v1.x` must keep resolving for the
      module's remaining consumer, so a bare version tag would be ambiguous in
      two directions.
- [x] 4.10 **Pulled forward rather than filed as a follow-up**, because the
      cutover is now a handoff rather than a GCP-to-GCP repoint: the image has to
      be pullable by an operator with no access to the GCP project that builds it
      today. `publish-api-image.yml` builds and pushes to `ghcr.io` on an
      `api/v*` tag or manual dispatch, with provenance and an SBOM, and writes
      the digest to the job summary so a handoff can quote it rather than
      rediscover it. Deploy the digest, not a tag.
      **Not verified:** no push has happened, so this workflow has never run.

## 5. Documentation and repository identity

- [x] 5.1 `docs/upstream-graft.md` rewritten around consolidation. Opens by
      saying what it used to claim and why that inverted, so a returning reader
      is not left reconciling two documents from memory. States which server
      ships and why; narrows the still-graftable list to `/capabilities` (**D7**)
      plus a deferred cache seam; and keeps a *Rationales that stopped holding*
      section rather than deleting the arguments that expired — including
      **D13**'s and "keep the handlers thin so they lift out cleanly", which now
      has nowhere to lift out to.
- [x] 5.2 `AGENTS.md` updated: four parts in a table, a "Which API server
      ships" section, a **Quarantined: do not "fix" these** section naming the
      `gs://` constants, the `x-google-backend` block, `cron/`'s GCS write path,
      and the scoped linter block. Plus an `api/` tree section covering the
      hand-owned `configure_scorecard.go`, the thin end-to-end coverage, and why
      the binary names are inverted.
- [x] 5.3 `openspec/config.yaml` context rewritten for the three code trees,
      which one ships, the enforced import-edge rule, the inbound direction of
      travel, and the quarantined violations.
- [x] 5.4 `README.md` gains a **Results API (api.scorecard.dev)** section —
      tree map, endpoints, make targets, the ghcr publish path — and its
      *About The Project* table now lists four parts with accurate states. The
      pre-existing "This server is an incubator, not a permanent fork" claim
      above the API-server section was false as written and is replaced with a
      note that it is not the deployment path.
- [x] 5.5 `internal/httpapi`'s package doc gains a **Status: not the deployment
      path** section — what ships instead, why, that the package is retained
      deliberately rather than abandoned, and that the honest options later are to
      converge or remove it rather than maintain it indefinitely.
- [x] 5.6 `internal/model`'s doc records that **half of D13's premise expired**:
      the generated models are no longer only upstream, and go-openapi is a direct
      dependency of this module now regardless. The remaining argument is narrower
      and stated as such — depending on those models would couple the cache path
      to a tree CI forbids importing.
- [ ] 5.7 **Remove the `api/` linter-exclusion block from `.golangci.yml` when
      the behavior freeze lifts**, and fix what it surfaces. The block is scoped
      to `api/` and annotated with a pointer to this task; it exists because the
      imported tree is frozen, not because the findings are wrong (**W7**, 3.5).

## 6. Production cutover

- [ ] 6.0 Capture the before-state of every external system (**W11**).
      **Script written, not run:** `scripts/cutover/capture-config.sh` covers
      Cloud Build triggers, the Cloud Run service and its revisions (the
      rollback targets), the deployed Endpoints config, domain mappings, DNS
      records with their TTLs, IAM, bucket metadata, and Secret Manager entries
      by name. It is best-effort per section — a permission failure records
      itself and the run continues, because a partial capture is useful and an
      aborted one is not — and it redacts probable inline credentials.
      **Unverified: this environment has no gcloud and no credentials**, so the
      resource names are informed guesses from `api/openapi.yaml`,
      `api/Dockerfile`, and `docs/dns.md`. Expect to correct some; the FAILED
      markers in its summary are where to look. Fastly and OSS-Fuzz are listed
      as explicitly manual.
      This is the task whose cost rises fastest with delay: none of it is in
      git, and it stops being readable when project access lapses.
      **First real run (2026-08-25) failed on expired gcloud credentials** —
      `Reauthentication failed. cannot prompt during non-interactive execution`
      — and exposed two defects in the script rather than anything about the
      project. Both fixed and tested against a stubbed `gcloud`:
      it now fails fast on missing credentials instead of reporting the same
      auth error fifteen times and burying the one instruction that fixes it;
      and it defaults to a timestamped, gitignored directory and refuses a
      repository root, because passing `.` scattered a dozen loose files into
      the working tree. Still blocked on an interactive `gcloud auth login`.
- [ ] 6.1 Build and publish the API image to a **staging tag**, never the tag the
      production service consumes.
- [ ] 6.2 Deploy a non-production Cloud Run revision with no traffic assigned.
- [ ] 6.3 Run the response diff against production (**W11**, step 3).
      **Harness built and exercised against production:**
      `scripts/api-conformance/` — 20 requests covering the two-bucket read
      fallback, absent and malformed input, path traversal, every documented
      badge style, and the CORS headers the website depends on. Compares status,
      body, and the cache headers; ignores per-deployment noise; records
      `age`/`x-cache` and prints them beside any body difference.
      Deliberately live-against-live rather than against a committed fixture:
      result bodies change on rescan, so a stored baseline is stale within a
      week and every later run drowns in false differences.
      **Calibrating it against the two production hostnames found a real
      defect** — see 6.3a. Run it against *origins*, not CDN hostnames.
- [ ] 6.3a **Finding: the two published hostnames serve different data.**
      `api.scorecard.dev` and `api.securityscorecards.dev` front the same
      service, but returned results for `github.com/ossf/scorecard` from scans a
      week apart (`date` 2026-08-22 vs 2026-08-15) — same resolved commit, same
      engine version, both Fastly `HIT` at different ages. `Surrogate-Control`
      is `max-age=31557600`, so an edge object survives a year unless purged,
      and `post_results.go` purges a single `API_BASE_URL`. A purge refreshes one
      hostname and leaves the other stale.
      Two consequences. Operationally this is a **pre-existing production
      defect**, not something the migration introduces: which hostname a
      consumer uses determines how fresh their results are. For the migration it
      means the cutover diff must run origin-to-origin, or it measures cache
      vintage rather than behavior. Worth fixing at the purge rather than
      carrying across; not in this change's scope.
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
