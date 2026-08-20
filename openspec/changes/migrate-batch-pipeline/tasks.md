# Tasks: Migrate the batch scanning pipeline out of ossf/scorecard

Groups map to the migration's phases. Each group is independently reviewable and
independently revertible. **Group 7 MUST NOT begin until group 6 has run clean
for at least one full scan cycle** (**C10**).

## 0. Pre-work / decisions

- [x] 0.1 **C6** resolved: the GitHub token-pool server moves, to
      `cron/internal/githubserver/`. Upstream builds no cron image after removal.
- [x] 0.2 **C7** resolved: pin the engine to a release with an automated bump,
      **plus** a scheduled canary building the pipeline against `ossf/scorecard`'s
      `main` (implemented in 4.7)
- [x] 0.3 **C8** resolved: accept cross-repo drift, fail the dependency bump hard
      on it, and fold the schema check into the C7 canary. Consolidating the
      duplicate `json.v2.schema` is a separate, non-blocking track.
- [x] 0.4 **C4** (tip-only import rewrite), **C5** (top-level `cron/`), and **C9**
      (redirect stub upstream, retained indefinitely) confirmed
- [x] 0.5 Cloud Build trigger ownership resolved: the change author holds `openssf`
      project admin. Remaining work is scheduling the cutover window (6.1) and
      capturing the triggers' before-state for rollback (6.0).
- [ ] 0.6 Verify there are no external consumers of
      `github.com/ossf/scorecard/v5/cron/data` or `.../cron/config` — GitHub code
      search plus `pkg.go.dev` importers. None are known or presumed; a positive
      result changes upstream removal to a deprecation window.
- [x] 0.7 `git-filter-repo` 2.47.0 installed; `--dry-run` confirmed `re` is
      available in the `--message-callback` context without an explicit import
      (**C1**)
- [ ] 0.8 Obtain approvals: Scorecard Steering Committee and at least one
      non-Steering Scorecard maintainer. Ask the committee whether it wants a
      standalone summary document rather than this OpenSpec change as the artifact.

## 1. History extraction, reviewed in isolation

**Nothing is pushed to `ossf/scorecard-infra` in this group.**

- [x] 1.1 Cloned `ossf/scorecard` fresh and single-branch to
      `../scorecard-cron-extract`; upstream tip `d1fab88f`, 3,104 commits —
      matching the proposal's analysis basis exactly (**C1**)
- [x] 1.2 Deleted all 48 tags in the clone before filtering (**C2**)
- [x] 1.3 Ran `git filter-repo` over `cron/` and
      `clients/githubrepo/roundtripper/tokens/server/`, renaming the latter to
      `cron/internal/githubserver/` and rewriting `(#N)` → `(ossf/scorecard#N)`
      (**C1**/**C3**/**C6**)
- [x] 1.4 Gate passed — `git log --follow -- cron/data/writer.go` resolves through
      13 commits into its pre-rename `cron/internal/data/writer.go` history.
      `cron/internal/githubserver/main.go` likewise follows back 11 commits
      through its relocation.
- [x] 1.5 Gate passed — **466 non-merge commits, an exact match**. Note the
      proposal's "~455" figure counted `cron/` alone; the union with the token
      server (76 commits, 65 overlapping) is 466. `git rev-list --count HEAD`
      reports 468 because `filter-repo` retains two 2020-era merge commits that
      pathspec history simplification hides — genuine history, not contamination.
- [x] 1.6 Gate passed — 131 files, all under `cron/` (128 pipeline + 3 relocated
      token server); nothing else survived the filter
- [x] 1.7 Gate passed — zero tags
- [x] 1.8 Rewrite verified — 0 bare `(#N)` references remain; 394 commits carry
      rewritten `(ossf/scorecard#N)` references (the remainder predate the
      squash-merge convention and never had one). History spans 2020-11-10 to
      2026-08-07; blame attributes to original authors.
- [ ] 1.9 Land the inbound-import CI guard in `ossf/scorecard` (fails on any
      non-`cron/` file importing `.../v5/cron/`); it is deleted in group 7
      (**C12**)

## 2. Graft into scorecard-infra

- [x] 2.1 Branch `import-batch-pipeline` created off `main` at `5082c25`; filtered
      history merged with `--allow-unrelated-histories`, conflict-free as
      predicted (this repo had no `cron/` path). Repo now at 517 commits.
- [x] 2.2 Verified — rename tracking survives the merge; `cron/data/writer.go`
      still resolves 13 commits back into pre-rename history
- [x] 2.3 Verified — `git blame` on `cron/internal/worker/main.go` attributes to
      original authors (Spencer Schrock, Azeem Shaikh, raghavkaul, Naveen, and
      others), not the merge commit
- [x] 2.4 Verified — zero tags arrived with the merge
- [x] 2.5 Extraction remote removed. Branch is **local and unpushed** — the
      filtered result at `../scorecard-cron-extract` and this branch are both
      reviewable before anything reaches the remote.
- [x] 2.6 Recorded the provenance of the build wiring that could **not** be
      imported — shared root files are fragments of engine-shared files and are
      hand-ported in 3.7 / 4.1–4.3. See `cron/initial-graft.md` (**C13**).
      All Dockerfiles, Cloud Build configs, and k8s manifests *did* import with
      history; only the `Makefile`, `docker.yml`, `main.yml`, `dependabot.yml`,
      `.codecov.yml`, and `cloudbuild/README.md` fragments did not.

## 3. Make it build

- [x] 3.1 Import paths rewritten in 26 Go files (`cea88c4`). The rewrite was
      Go-only in practice: the only other files carrying the old path are the four
      that *describe* the migration, which must keep it. The `ko` recipes noted in
      **C13** were written fresh against the new path in 3.7 rather than rewritten.
- [x] 3.2 `go_package` corrected in both `.proto` files (`b63e3a3`).
      **`.pb.go` regeneration deliberately deferred** — the `go_package` string is
      embedded in each file's `rawDesc` descriptor bytes, so regeneration is a real
      change, but protoc is absent, upstream pins neither protoc nor
      `protoc-gen-go` at the call site, and the checked-in files came from
      `protoc-gen-go v1.28.1` while upstream's `tools/go.mod` now resolves
      `v1.36.11`. Regenerating now would mix a one-line fix with eight minor
      versions of pre-existing generator drift. Do it with `make build-proto`.
- [x] 3.3 `go mod tidy` clean (`b63e3a3`). Engine requirement is `v5.5.0`, already
      an explicit release pin per **C7**. Pipeline dependencies promoted to direct:
      BigQuery, Pub/Sub, Stackdriver exporter, csvutil, go-jsonschema-generator,
      gojsonschema, OpenCensus, protobuf, release-utils. **New indirect surface
      worth noting:** `aws-sdk-go` v1 and Prometheus, arriving via the
      BigQuery/Arrow and Stackdriver chains rather than from pipeline code.
- [x] 3.4 Lint reconciled — 10 issues, all `gci`/`gofmt` import ordering caused by
      the module prefix change, auto-fixed. The `.golangci.yml` `gci` prefix
      already pointed at this module; no config change was needed and no linter
      was disabled for the imported tree.
- [x] 3.5 Verified clean both directions — `cron/` imports nothing from
      `internal/`, and neither `internal/` nor `cmd/` imports `cron/` (**C11**)
- [x] 3.6 `go build ./...`, `go test ./... -race`, and `golangci-lint run ./...`
      all pass with the pipeline included. `cron/internal/format`'s
      `Test_GenerateBQSchema` and `Test_GenerateJSONSchema` pass against the pinned
      engine — **C8**'s drift check is green at `v5.5.0`.
- [x] 3.7 Makefile written (`35c69e0`): six docker + six ko image targets, seven
      pipeline binaries, `build-proto`, `add-projects`, `validate-projects`, and
      `build`/`test`/`lint`. Hand-written against upstream's, not imported
      (**C13**); provenance in `cron/initial-graft.md`. Three deliberate
      divergences, all recorded in the commit and in file comments: build tools are
      expected on PATH rather than vendored (a fresh `tools/` module does not
      resolve, and `tool` directives would put ko's tree in the main module's
      graph); protobuf generation is explicit rather than a file rule; `LDFLAGS` is
      inlined rather than shelling out to a script goreleaser would have shared.
      Verified: all seven binaries build, all three verify targets pass, and
      `validate-projects` runs clean against all three real inventories.
- [ ] 3.8 Merge the import PR — [#27](https://github.com/ossf/scorecard-infra/pull/27),
      open, covering groups 2–5 as a single review.
      **Every check passes except DCO** (see 3.9), including all six image builds,
      `build`/`test`/`lint`, both inventory jobs, `zizmor`, and Kusari.
- [ ] 3.9 **Resolve the DCO block — decision needed, no code fix exists.**
      The DCO app checks every commit in a pull request. 251 of the 475 non-merge
      commits lack a `Signed-off-by` trailer. All 251 are *imported upstream
      history* (largely dependabot); all 12 authored commits are signed.
      This is a structural consequence of grafting history into a DCO-gated
      repository, and the migration plan did not anticipate it.
      **Adding sign-off to those commits is not an option.** The DCO is a legal
      certification made by a commit's author; retroactively adding a
      `Signed-off-by` on someone else's behalf forges that attestation. It is also
      unnecessary — these commits are already public in `ossf/scorecard` under
      Apache-2.0, and importing them does not change their provenance.
      Viable paths: merge with an administrative override (the check is
      informational unless required); or configure the DCO app to scope its check.
      Squashing the import would satisfy DCO by destroying the change's purpose.

## 4. CI and image builds, in parallel with production

**Production still runs images built from `ossf/scorecard` throughout this group.**

- [x] 4.1 `.github/workflows/build-images.yml` added — a six-target matrix over the
      pipeline's docker targets, with upstream's docs-only short-circuit (worth
      keeping here: this repo is markdown-heavy). Scoped to the pipeline only;
      upstream's `scorecard-docker` and `build-attestor-docker` entries are engine
      targets and stay there. `actionlint` and `zizmor` both clean.
      **Verified in CI (PR #27): all six images build.** The local failure was
      environmental (TLS interception breaking module downloads inside build
      containers), not a defect.
      CI also caught a real defect local builds could not: relocating the token
      server (**C6**) moved its files but not the paths *inside* them — its
      Dockerfile still copied from and set `ENTRYPOINT` to the old
      `clients/githubrepo/...` path, and its `cloudbuild.yaml` pointed `-f` at the
      old Dockerfile location. Go imports fail to compile when stale; strings in a
      Dockerfile do not, so only an image build finds them. Fixed in `80a45d8`.
      The `ko` targets are in the Makefile but are **not** wired into CI: they
      duplicate the docker matrix's coverage and their value is at publish time,
      which is Cloud Build's job. Wire them if a ko-based publish path is chosen.
- [x] 4.2 `add-projects` / `validate-projects` jobs added to `presubmits.yml`.
      Verified locally that `add-projects` is a genuine no-op on the current
      inventory, so the `git diff --exit-code` guard passes rather than failing on
      first run. Unlike upstream these jobs need no protoc, because generation is
      no longer a file rule (**C13**).
- [x] 4.3 Six Dockerfile directories added to `.github/dependabot.yml` — one more
      than the plan's five, because the token-pool server's Dockerfile moved into
      `cron/` with the rest of the pipeline (**C6**). YAML validated.
- [ ] 4.4 Publish images to a **staging tag or registry path** — never the
      `:latest` / `:stable` tags that `cron/k8s/*.yaml` consume.
      **Blocked on an ops decision, not on code.** Confirmed tag topology: the
      Cloud Build configs push `:$COMMIT_SHA` and `:latest`, while the k8s
      manifests consume `:stable`. A staging path needs a registry destination
      chosen (separate project? `gcr.io/openssf/staging-*`?) before
      `cron/cloudbuild/*.yaml` can be varied for it. Deliberately not invented here.
- [ ] 4.5 Diff each staging-built image against its production equivalent to
      confirm the split introduced no behavioral change. Depends on 4.4.
- [ ] 4.6 Measure the CI runtime increase; if the presubmit path is now
      unreasonable, split image builds into their own workflow. Requires real runs;
      note image builds already live in their own workflow, so the mitigation is
      partly pre-applied.
- [x] 4.7 `.github/workflows/canary.yml` added — daily at 06:17 UTC plus manual
      dispatch, pointing the engine dependency at `main` and running the full
      build and test suite. That suite includes `cron/internal/format`'s
      `Test_GenerateBQSchema` / `Test_GenerateJSONSchema`, so the job is both the
      **C7** breakage canary and the **C8** drift check. It runs on `schedule` and
      `workflow_dispatch` only, never on `pull_request`, so it cannot gate
      unrelated work. `actionlint` and `zizmor` clean.
- [ ] 4.8 Route canary failures somewhere a maintainer reads. The workflow
      documents what a sustained red means (talk to the engine maintainers *before*
      the next bump), but **no notification is wired** — a scheduled job's failures
      are invisible unless someone opens the Actions tab. Needs a destination
      chosen: issue-on-failure, a chat webhook, or repo notification settings.
      Until this is done the canary is decorative.
- [x] 4.9 Satisfied by construction rather than by new configuration: Dependabot
      opens engine bumps as pull requests, `presubmits.yml`'s test job runs on
      every pull request, and that job runs the schema verification tests. A bump
      that drifts the schema therefore fails as a red required check, not a
      warning. No separate mechanism is needed; the requirement is that the check
      stays a *blocking* one, which it is.

## 5. Documentation and repository identity

- [x] 5.1 `AGENTS.md` rewritten to describe two adjacent systems rather than one,
      with a `cron/` component map, the behavior-freeze rule, the schema-contract
      warning, and the Makefile-based workflow. Carries an explicit "do not tidy
      this by wiring the two together" instruction (**C11**) — the failure mode is
      a future agent treating the missing import edges as an oversight.
- [x] 5.2 `docs/upstream-graft.md` gains a section stating the pipeline is the one
      component travelling **inbound**, plus a `cron/` row in the component graft
      map marked "does not graft back". Positioned before the graft rules so a
      reader meets the exception before the rules it contradicts.
- [x] 5.3 `openspec/config.yaml` context now describes both systems, the C11
      boundary, and the inbound direction of travel.
- [x] 5.4 `README.md` gains a **Batch scanning pipeline** section: component and
      image table, deployment manifest locations, the schema-contract owner, the
      scan inventories with their contribution workflow, and the pipeline make
      targets. Includes a note that the inventories moved from `ossf/scorecard`, so
      someone arriving on a stale link lands on an explanation (**C9** —
      the upstream half of that redirect is task 7.4).

## 6. Production cutover

- [ ] 6.0 Capture the Cloud Build triggers' current configuration before changing
      it, so rollback is a known state rather than a reconstruction — this config
      is not in git and the cutover is not revertible by `git revert` (**C10**)
- [ ] 6.1 Repoint the Cloud Build triggers in the `openssf` GCP project from
      `ossf/scorecard` to `ossf/scorecard-infra`
- [ ] 6.2 Run a full pipeline cycle end to end: controller → PubSub → worker → GCS
      → BigQuery transfer → webhook
- [ ] 6.3 Compare output row counts and schema against the prior cycle
- [ ] 6.4 Notify the Scorecard community meeting before this cutover
- [ ] 6.5 Hold for at least one full scan cycle before group 7. **Rollback:**
      repoint the triggers back — the code still exists upstream, so no code
      restoration is needed (**C10**)

## 7. Removal from ossf/scorecard

- [ ] 7.1 Notify the Scorecard community meeting again before removal — the
      inventory contribution path changes for everyone
- [ ] 7.2 Delete `cron/` and `clients/githubrepo/roundtripper/tokens/server/`
- [ ] 7.3 Strip the Makefile targets, `docker_matrix` entries, `add-projects` /
      `validate-projects` jobs, Dependabot paths, and the `.codecov.yml` ignore
- [ ] 7.4 Leave a redirect stub at `cron/README.md` pointing to the new location
      (**C9**)
- [ ] 7.5 Rewrite the `CONTRIBUTING.md` "dailyscore-cronjob" section to point at
      `ossf/scorecard-infra`, with explicit new URLs for `projects.csv` and
      `gitlab-projects.csv`
- [ ] 7.6 Update the `AGENTS.md` repository layout tree and `cloudbuild/README.md`
- [ ] 7.7 Add an issue-template callout so misdirected inventory PRs get
      redirected rather than closed
- [ ] 7.8 Remove the inbound-import CI guard added in 1.9 (**C12**)
- [ ] 7.9 Confirm `clients/githubrepo/roundtripper/tokens/` is untouched and
      `roundtripper.go` / `transport.go` still build
- [ ] 7.10 Confirm `ossf/scorecard` builds, tests, and lints clean with no
      remaining pipeline references

## 8. Change closeout

- [ ] 8.1 `openspec validate migrate-batch-pipeline --strict` passes
- [ ] 8.2 Verify every success criterion in the proposal and design is met
- [ ] 8.3 Archive the change once implemented and merged
