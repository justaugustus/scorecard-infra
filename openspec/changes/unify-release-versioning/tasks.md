# Tasks: One release line for the whole repository

## 1. Decision record

- [x] 1.1 Write this change: proposal, design (**V1**–**V5**), and the
      `release-versioning` spec.
- [x] 1.2 Amend `migrate-api`: mark **W15** step 2 superseded in `design.md`,
      note it in `proposal.md`, and replace the component-scoped requirement in
      `specs/api-deployment/spec.md` so the two changes do not contradict each
      other when `migrate-api` archives.

## 2. Publishing

- [x] 2.1 `git mv publish-cron-images.yml publish-images.yml`, and add the API
      image to its matrix (**V3**).
- [x] 2.2 Trigger on `v*` tags; derive the image version by stripping the `v`.
- [x] 2.3 Skip `:latest` for pre-release tags (**V4**).
- [x] 2.4 Label every image with `org.opencontainers.image.version`,
      `dev.scorecard.engine.version`, and `dev.scorecard.engine.commit`
      (**V5**).
- [x] 2.5 Remove `publish-api-image.yml`.
- [x] 2.6 Update the docs that name `api/v*`, `cron/v*`, or the old workflows:
      `api/README.md`, `cron/k8s/README.md`,
      `deploy/api/environments/production/variables.tf`,
      `deploy/api/modules/service/variables.tf`, and `build-images.yml`'s
      header.
- [x] 2.7 `actionlint` and `zizmor` clean.

## 3. First release

- [x] 3.1 Push a pre-release tag (e.g. `v0.2.0-rc.1`) to a fork. Confirm all
      seven images publish to the fork's `ghcr.io` namespace with the version
      tag and the three labels, and that `:latest` does not move.
      **Done 2026-10-14:** `v0.2.0-rc.1` at `090dc48` on the `justaugustus`
      fork published all seven images as `:0.2.0-rc.1`, labelled
      `v0.2.0-rc.1` / `v5.6.0` / `61e84743`, with no `:latest`.
- [x] 3.2 Tag `v0.2.0`. The release notes say it supersedes `api/v1.1.0` and
      `cron/v*`, that it now covers the batch pipeline, why the line is v0, and
      which engine version it runs.
      **Done 2026-10-14, differently:** a signed tag at `090dc48` with no
      GitHub release or notes; this change's design records the rest. All
      seven images published with `:0.2.0` and `:latest`.
- [x] 3.3 Deploy the API by the `v0.2.0` digest, keeping the previous digest as
      the rollback target.
      **Done 2026-10-14:** task definition revision 3 runs
      `scorecard-api@sha256:ee261dd8…`; the rollback target is
      `sha256:bc3b3e53…`. #133's publish with `cache-mode: none` landed.
