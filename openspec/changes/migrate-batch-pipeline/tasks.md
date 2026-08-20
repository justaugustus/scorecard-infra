# Tasks: Migrate the batch scanning pipeline out of ossf/scorecard

Groups map to the migration's phases. Each group is independently reviewable and
independently revertible. **Group 7 MUST NOT begin until group 6 has run clean
for at least one full scan cycle** (**C10**).

## 0. Pre-work / decisions

- [ ] 0.1 Resolve **C6**: confirm the GitHub token-pool server moves at all, and
      confirm `cron/internal/githubserver/` as its destination
- [ ] 0.2 Resolve **C7**: pin the Scorecard engine to a release (recommended, with
      an automated bump) or track `main` — decided by the cron operations owner
- [ ] 0.3 Resolve **C8**: confirm the dependency bump fails hard on schema drift,
      and whether consolidating the duplicate `json.v2.schema` is a prerequisite
      or a follow-on
- [ ] 0.4 Confirm **C5** (top-level `cron/`), **C4** (tip-only import rewrite), and
      **C9** (redirect stub upstream) with reviewers
- [ ] 0.5 Identify who holds Cloud Build trigger edit rights in the `openssf` GCP
      project; confirm availability for the cutover window
- [ ] 0.6 Check for known external consumers of
      `github.com/ossf/scorecard/v5/cron/data` and `.../cron/config`; if any
      exist, plan a deprecation window instead of a delete
- [ ] 0.7 Install `git-filter-repo` (`brew install git-filter-repo`) and confirm
      `re` is available in its `--message-callback` context on a dry run (**C1**)
- [ ] 0.8 Obtain approvals: Scorecard Steering Committee, `openssf` GCP project
      admin (on the trigger repointing specifically), and at least one
      non-Steering Scorecard maintainer

## 1. History extraction, reviewed in isolation

**Nothing is pushed to `ossf/scorecard-infra` in this group.**

- [ ] 1.1 Clone `ossf/scorecard` fresh and single-branch into a disposable
      directory — never run the filter against a working checkout (**C1**)
- [ ] 1.2 Delete all tags in the clone before filtering (**C2**)
- [ ] 1.3 Run `git filter-repo` over `cron/` and
      `clients/githubrepo/roundtripper/tokens/server/`, renaming the latter to
      `cron/internal/githubserver/` and rewriting `(#N)` → `(ossf/scorecard#N)`
      in commit messages (**C1**/**C3**/**C6**)
- [ ] 1.4 Gate — rename tracking: `git log --follow` on a historically renamed
      file (e.g. `cron/data/writer.go`) resolves into pre-rename history
- [ ] 1.5 Gate — commit count matches the pre-filter analysis (~455)
- [ ] 1.6 Gate — no path outside the pipeline tree survived the filter
- [ ] 1.7 Gate — zero tags present
- [ ] 1.8 Publish the filtered result to a scratch repository or share the bundle
      for maintainer review before any graft
- [ ] 1.9 Land the inbound-import CI guard in `ossf/scorecard` (fails on any
      non-`cron/` file importing `.../v5/cron/`); it is deleted in group 7
      (**C12**)

## 2. Graft into scorecard-infra

- [ ] 2.1 Create the import branch and merge the filtered history with
      `--allow-unrelated-histories`, with a merge message recording the commit
      count, date range, mechanism, and the PR-reference rewrite
- [ ] 2.2 Verify `git log --follow` on a historically renamed file resolves
      through the merge
- [ ] 2.3 Verify `git blame` on `cron/internal/worker/main.go` attributes to
      original authors, not the merge commit
- [ ] 2.4 Verify no `ossf/scorecard` tags arrived with the merge
- [ ] 2.5 Remove the extraction remote; open as a draft PR — **do not merge**,
      group 3 lands on the same branch

## 3. Make it build

- [ ] 3.1 One commit rewriting `github.com/ossf/scorecard/v5/cron/...` →
      `github.com/ossf/scorecard-infra/cron/...` across the tree (**C4**)
- [ ] 3.2 Correct `go_package` in `cron/data/request.proto` and
      `cron/data/metadata.proto` (both currently declare the stale
      `github.com/ossf/scorecard/cron/data`, missing even `/v5`) and regenerate
      the `.pb.go` files
- [ ] 3.3 `go mod tidy`; confirm the `github.com/ossf/scorecard/v5` requirement
      resolves everything the pipeline needs, and apply the **C7** decision
      (pinned release + automated bump, or pseudo-version)
- [ ] 3.4 Reconcile upstream's `.golangci.yml` rules with this repo's; resolve new
      lint failures by adjusting config or code, not by exempting the tree
- [ ] 3.5 Confirm no import edge exists in either direction between `cron/` and
      this repo's `internal/` packages (**C11**)
- [ ] 3.6 `go build ./...`, `go test ./... -race`, and `golangci-lint run ./...`
      clean with the pipeline included
- [ ] 3.7 Port the Makefile targets — build, docker, ko, proto, add-projects,
      validate-projects — adjusting paths for this repository root
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

## 5. Documentation and repository identity

- [ ] 5.1 Update `AGENTS.md`: the repository is now an API server **and** the batch
      pipeline; document the imported tree and its boundary (**C11**)
- [ ] 5.2 Update `docs/upstream-graft.md`: the batch pipeline is explicitly **not**
      a graft target — its direction of travel is inbound
- [ ] 5.3 Update `openspec/config.yaml` context to reflect the widened scope
- [ ] 5.4 Document the pipeline's operational entry points (build, deploy, the
      inventory contribution path) in `README.md` or a dedicated doc

## 6. Production cutover

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
