# Tasks: Migrate the batch scanning pipeline out of ossf/scorecard

Groups map to the migration's phases. Each group is independently reviewable and
independently revertible. Group 6 closed out void, without running, when the
`openssf` GCP project was turned down (see the closeout note under that group
and design.md's **C10**). Group 7 no longer waits on it; it is gated on its own
7.1 alone.

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
- [x] 0.6 **Verified: no external Go importers.** `pkg.go.dev` reports 0 imported-by
      for both `cron/data` and `cron/config`. GitHub code search returns only
      `ossf/scorecard` (the source), `ossf/scorecard-infra` (this change's own
      docs), and `epwqicelhf/oss-ecosystem-security` — which contains a vendored
      file copy under `reference/scorecard-main/`, not a module dependency, and so
      is unaffected by removal. **Clean delete is safe**; the deprecation-window
      path in the `project-inventory` spec does not trigger.
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
- [x] 3.2 `go_package` corrected in both `.proto` files (`b63e3a3`) **and the
      `.pb.go` files regenerated** via `make build-proto` with `protoc-gen-go`
      pinned to `v1.36.11` — the version upstream's own `tools/go.mod` resolves, so
      the output matches what upstream would produce today rather than what it
      produced in 2022. `.proto` and generated output are now consistent.
      The diff is 73+/144− across both files and is mostly generator
      modernization (field reordering, `protogen:"open.v1"` tags, an `unsafe`
      import) rather than the one-line semantic change; that pre-existing drift is
      the cost of the checked-in output being eight minor versions stale.
      Public API and wire format are unchanged; tests and lint pass.
      `protoc` itself is unpinned upstream (`$(shell which protoc)`), so its
      recorded version tracks whatever the regenerating machine has — here v7.35.1
      against upstream's v3.21.6. Regenerate with `protoc-gen-go v1.28.1` instead
      if reviewers prefer the minimal diff over toolchain currency.
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
- [x] 3.8 **Reconciled 2026-08-30: landed by a different mechanism than
      named.** Groups 2–5 are in `main` via merge commit `88c818c` ("Import
      cron batch infrastructure from ossf/scorecard"), pushed directly rather
      than merged through
      [#27](https://github.com/ossf/scorecard-infra/pull/27) — #27 is
      **closed, unmerged**. The task closes on its outcome (the tree is in
      `main`, history intact) rather than the specific route it named.
- [x] 3.9 **Resolved by the route 3.8 took, 2026-08-30.** The import landed by
      direct push to `main`, not by merging the DCO-gated pull request, so the
      251 unsigned imported-history commits were never evaluated against the
      DCO check at all — no override was needed on them, forged, or
      configured around. This is the "administrative override" path this task
      named, applied at the push rather than at the PR-merge step, which is
      also why #27 closes unmerged instead of merged. No commit was altered;
      no attestation was forged.

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
      **Resolved for the destination registry (2026-08-28):**
      `.github/workflows/publish-cron-images.yml` publishes all six to
      `ghcr.io/<owner>/scorecard-*`, keeping the names Cloud Build already
      produces so the `cron/k8s/*.yaml` edit stays a registry prefix rather than
      a rename — which is what the freeze requires. Merges publish a mutable
      `main` tag, `cron/v*` publishes immutable semver and `latest`, and the
      owner is parameterized so a fork can exercise it against its own
      namespace. This does not close 4.4: `:stable`, which the manifests
      actually consume, is still nobody's decision, and GHCR does not answer the
      question of what promotes an image to it.
- [ ] 4.4a **Anomaly: `cron/k8s/webhook.release.yaml` names an image nothing
      builds.** It refers to `scorecard-webhook-releasetest`, while every other
      release-test workload reuses its normal image under a `-releasetest`
      *deployment* name — `controller.release.yaml` and `worker.release.yaml`
      both run `scorecard-batch-controller:latest` and
      `scorecard-batch-worker:latest` against different topics, buckets, and the
      `gitlab-projects-releasetest.csv` project list. So the webhook reference is
      inconsistent with its neighbours, and no Makefile target, Cloud Build
      config, or CI job in this repository has ever produced that image.
      **Deliberately not resolved either way.** Publishing a seventh image would
      mint an artifact to match what is probably a mistake; correcting the
      manifest edited a frozen tree. Recorded so it is a known gap rather than
      something rediscovered as a broken deploy.
      Whether that release-test environment still runs at all is worth
      establishing first — a manifest referencing an unbuildable image suggests
      it may not.
      **Update, 2026-08-30:** the freeze this task deferred to no longer
      applies — it lifted with group 6's closeout (see above) — so editing the
      manifest is no longer blocked on that. This stays open because the
      harder question is unchanged: whether the release-test environment
      exists at all is still unconfirmed, and fixing the manifest before that
      is answered risks correcting a reference nobody uses.
- [ ] 4.5 Diff each staging-built image against its production equivalent to
      confirm the split introduced no behavioral change. Depends on 4.4.
- [x] 4.6 **Measured from PR #27's runs. The presubmit path did not slow down.**
      `presubmits` wall clock is unchanged at ~212s: the two new inventory jobs
      (142s, 113s) run in parallel and finish before `test` (212s), which was
      already the critical path. Image builds add a separate ~377s path
      (`cron-controller-docker`, the slowest of six).
      Net effect: PR feedback goes from ~3.5min to ~6.3min wall clock, while total
      compute roughly triples (~594s → ~1878s). The mitigation this task
      anticipated — moving image builds out of the presubmit path — was applied up
      front, so nothing further is needed. If the ~6.3min becomes the complaint,
      the lever is matrix parallelism or layer caching, not workflow splitting.
- [x] 4.7 `.github/workflows/canary.yml` added — daily at 06:17 UTC plus manual
      dispatch, pointing the engine dependency at `main` and running the full
      build and test suite. That suite includes `cron/internal/format`'s
      `Test_GenerateBQSchema` / `Test_GenerateJSONSchema`, so the job is both the
      **C7** breakage canary and the **C8** drift check. It runs on `schedule` and
      `workflow_dispatch` only, never on `pull_request`, so it cannot gate
      unrelated work. `actionlint` and `zizmor` clean.
- [x] 4.8 **Notification wired.** A `report-failure` job (`if: failure()`,
      `issues: write`) opens a tracking issue when the canary fails, and comments
      on the existing one rather than filing a new issue per run — a daily job that
      files a daily issue trains people to mute it. The issue body states plainly
      that the failure does *not* mean this repository is broken (pull requests
      build against the pinned release) and what to do about it: raise it with the
      engine maintainers before the next bump, while the change is still theirs to
      adjust. Issue-on-failure was chosen over a chat webhook because it needs no
      secret and leaves a durable, assignable record. The heredoc and
      lookup-or-create logic were dry-run locally with a stubbed `gh` — a
      notification path that only executes on failure is the worst place to
      discover a shell bug. `actionlint` and `zizmor` clean.
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

> **Closed out 2026-08-30 — void, not executed.** The `openssf` GCP project
> this group cuts over inside of was turned down before any task below ran.
> Design.md's **C10** section records why the tasks stay unchecked rather
> than being rewritten for AWS: the proof they encode needs a live
> production comparison this pipeline has never had outside GCP. Standing up
> and proving an AWS pipeline is scoped to a new, separate change.

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

> **Re-gated 2026-08-30.** This group no longer waits on group 6 (void, see
> above). Upstream's `cron/` and the GCP resources it names died with the
> `openssf` project turndown, so deleting it removes no working function —
> the gate that mattered was never letting the deletion outrun a proven
> replacement, and there is nothing left to outrun. The only remaining gate
> is 7.1, the community notice: the inventory contribution path changes for
> everyone who edits `projects.csv`, and that is a communication problem, not
> a technical one.

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
