# Proposal: Lift the migration quarantine

## Why

`migrate-api` and `migrate-batch-pipeline` froze the imported trees and
quarantined their provider coupling (**W10**, **C11**). The freeze kept each
cutover's acceptance test checkable: the deployed behavior did not change. The
import-edge ban kept `api/` byte-comparable to what production ran.

Both cutovers are done. The results API serves from AWS, the batch pipeline has
run on AWS since 2026-09-06, and the GCP project was shut down on 2026-08-30.
Nothing is being compared against a GCP deployment any more. But the guidance
still tells contributors and agents not to touch `api/` and `cron/`, and CI still
enforces an isolation that only existed for the freeze.

## What Changes

- **Remove the quarantine and freeze language** wherever it still reads as
  current: `openspec/config.yaml`, `README.md`, `CONTRIBUTING.md`,
  `SUPPORT.md`, `docs/upstream-graft.md`, `.codecov.yml`,
  `.golangci.yml`, the pull request template, and code comments in
  `publish-images.yml` and `internal/model`. (`AGENTS.md` was already pruned.)
- **Lift the import-edge rule** (**L2**). Delete `presubmits.yml`'s
  `import-edges` job and the PR template's checkbox. Remove the isolation
  requirements from `migrate-api`'s and `migrate-batch-pipeline`'s spec deltas,
  so archiving those changes doesn't make them canonical.
- **Remove the quarantine requirement** from `migrate-api`'s `hosted-api` spec
  delta, and mark **W10**'s constraints lifted in its proposal and design.
- **Remove `api/openapi.yaml`'s `x-google-backend` block** (**L4**), in its own
  commit, and regenerate the embedded spec with the pinned go-swagger.

## Capabilities

- **imported-trees** (new) — `api/` and `cron/` are held to the same rules as
  the rest of the repository.

## Impact

- `.github/workflows/presubmits.yml`, `.github/workflows/publish-images.yml`,
  `.github/PULL_REQUEST_TEMPLATE.md`
- `.codecov.yml`, `.golangci.yml` (comment only), `README.md`,
  `CONTRIBUTING.md`, `SUPPORT.md`, `docs/upstream-graft.md`,
  `openspec/config.yaml`, `internal/model/doc.go` (comment only)
- `api/openapi.yaml`, `api/app/generated/restapi/embedded_spec.go`
- `openspec/changes/migrate-api/`, `openspec/changes/migrate-batch-pipeline/`

No change to how either system behaves or deploys.

## Non-goals

- **Removing `.golangci.yml`'s `api/` linter block.** Its condition is now met,
  but removing it means fixing what it surfaces in `api/`. That is `migrate-api`
  task 5.7, a separate change.
- **Converging the two serving-tier implementations.** CI no longer prevents
  it, but it still deserves its own change.
- **Archiving `migrate-api` or `migrate-batch-pipeline`.** Their open tasks
  need their own triage.
- **Rewriting historical records** (**L3**): `*/initial-graft.md`, completed
  task notes, and archived changes stay as written.
