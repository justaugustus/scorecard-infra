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
      hand-ported in 3.7 / 4.1–4.3. See `docs/cron-build-provenance.md` (**C13**).
      All Dockerfiles, Cloud Build configs, and k8s manifests *did* import with
      history; only the `Makefile`, `docker.yml`, `main.yml`, `dependabot.yml`,
      `.codecov.yml`, and `cloudbuild/README.md` fragments did not.

## 3. Make it build

- [ ] 3.1 One commit rewriting `github.com/ossf/scorecard/v5/cron/...` →
      `github.com/ossf/scorecard-infra/cron/...` across the tree (**C4**). **Not
      only `.go` files** — the ported `ko` recipes embed the module path too
      (**C13**)
- [ ] 3.2 Correct `go_package` in `cron/data/request.proto` and
      `cron/data/metadata.proto` (both currently declare the stale
      `github.com/ossf/scorecard/cron/data`, missing even `/v5`) and regenerate
      the `.pb.go` files
- [ ] 3.3 `go mod tidy`; confirm the `github.com/ossf/scorecard/v5` requirement
      resolves everything the pipeline needs, and pin it to a release per **C7**
- [ ] 3.4 Reconcile upstream's `.golangci.yml` rules with this repo's; resolve new
      lint failures by adjusting config or code, not by exempting the tree
- [ ] 3.5 Confirm no import edge exists in either direction between `cron/` and
      this repo's `internal/` packages (**C11**)
- [ ] 3.6 `go build ./...`, `go test ./... -race`, and `golangci-lint run ./...`
      clean with the pipeline included
- [ ] 3.7 Port the Makefile targets — build, docker, ko, proto, add-projects,
      validate-projects — adjusting paths for this repository root. These are
      hand-ported, not imported: they are fragments of a file shared with the
      engine. Provenance is recorded in `docs/cron-build-provenance.md` (**C13**).
      Note there is no `.ko.yaml` content to port — upstream's has no cron entries.
- [ ] 3.8 Merge the group 2 + group 3 PR

## 4. CI and image builds, in parallel with production

**Production still runs images built from `ossf/scorecard` throughout this group.**

- [ ] 4.1 Port the image build matrix and `ko` targets into `presubmits.yml` or a
      new `docker.yml`; lint the workflows with `actionlint` and `zizmor`
- [ ] 4.2 Port the `add-projects` / `validate-projects` jobs
- [ ] 4.3 Add the five pipeline Dockerfile paths to `.github/dependabot.yml`
- [ ] 4.4 Publish images to a **staging tag or registry path** — never the
      `:latest` / `:stable` tags that `cron/k8s/*.yaml` consume
- [ ] 4.5 Diff each staging-built image against its production equivalent to
      confirm the split introduced no behavioral change
- [ ] 4.6 Measure the CI runtime increase; if the presubmit path is now
      unreasonable, split image builds into their own workflow
- [ ] 4.7 Add the **scheduled `main` canary** (**C7**/**C8**): build and test the
      pipeline against `ossf/scorecard`'s `main`, including `schema_gen_test.go`
      so data-model drift surfaces on the engine's cadence. **Non-blocking** on
      this repo's pull requests — it tests an upstream branch and must not gate
      unrelated work
- [ ] 4.8 Route canary failures somewhere a maintainer reads, and write down what
      a sustained red canary means (talk to engine maintainers before the next
      bump, not after). An unread canary manufactures confidence
- [ ] 4.9 Configure the engine dependency bump to **fail hard** on schema
      verification failure — a build break, never a warning on a dependency PR

## 5. Documentation and repository identity

- [ ] 5.1 Update `AGENTS.md`: the repository is now an API server **and** the batch
      pipeline; document the imported tree and its boundary (**C11**)
- [ ] 5.2 Update `docs/upstream-graft.md`: the batch pipeline is explicitly **not**
      a graft target — its direction of travel is inbound
- [ ] 5.3 Update `openspec/config.yaml` context to reflect the widened scope
- [ ] 5.4 Document the pipeline's operational entry points (build, deploy, the
      inventory contribution path) in `README.md` or a dedicated doc

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
