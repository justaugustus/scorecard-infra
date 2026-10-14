# Proposal: One release line for the whole repository

## Why

Releases are tagged per component: `api/vX.Y.Z` for the results API and
`cron/vX.Y.Z` for the batch pipeline (`migrate-api` **W15**). In practice:

- **Both systems build from one Go module**, so they always share an engine
  version. Two version lines suggest an independence that does not exist, and an
  engine bump needs two releases.
- **Only one line was ever used.** `api/v1.1.0` is the only tag; `cron/v*` has
  never been cut.
- **Nobody consumes the numbers externally.** Deployments pin digests, so a
  tag's job is to name a release, and one line does that.

The trigger matters now because a fix is waiting on it. `cache-mode: none` in a
workflow makes `api.scorecard.dev` reject the results upload with HTTP 400
([ossf/scorecard#5322](https://github.com/ossf/scorecard/issues/5322)). The
verifier fix is on `main`
([#126](https://github.com/ossf/scorecard-infra/pull/126)) but has never been
released. Switching the release line first means that fix ships as the first
release under the new scheme rather than as one more `api/v*` tag.

## What Changes

- **One repository-wide SemVer line, `vX.Y.Z`, starting at `v0.2.0`**, replacing
  `api/v*` and `cron/v*` for new releases (**V1**, **V2**).
- **One tag publishes every image** from that commit, from one workflow,
  `publish-images.yml`. It is `publish-cron-images.yml` renamed, with the API
  image added to its matrix; `publish-api-image.yml` is removed (**V3**).
  Each system still deploys separately, by digest.
- **Pre-release tags do not move `:latest`** (**V4**).
- **The engine version is recorded, not encoded.** Every image carries
  `dev.scorecard.engine.version`, `dev.scorecard.engine.commit`, and
  `org.opencontainers.image.version`; release notes name the engine version
  (**V5**).
- **Amend `migrate-api`**, whose component-scoped tag requirement this reverses.

## Capabilities

- **release-versioning** (new) — how releases are named and what a release
  publishes.

## Impact

- `.github/workflows/publish-images.yml` (renamed from
  `publish-cron-images.yml`), `publish-api-image.yml` (removed),
  `build-images.yml` (comment)
- `api/README.md`, `cron/k8s/README.md`,
  `deploy/api/environments/production/variables.tf`,
  `deploy/api/modules/service/variables.tf`
- `openspec/changes/migrate-api/` (design, proposal, `api-deployment` spec)

No change to any image's contents, the API contract, or how either system is
deployed.

## Non-goals

- **Deleting `api/v1.1.0`.** It stays; it names what it always named.
- **A version endpoint.** Planned separately; this change provides the labels it
  will report.
- **Committing the deployed production digest.** Worth doing so that each deploy
  is a reviewable change, but it is its own change.
- **Moving `cron/k8s/*.yaml` off mutable tags.** Still `migrate-batch-pipeline`
  task 4.4.
- **v1.** Leaving v0 is a separate, later decision.
