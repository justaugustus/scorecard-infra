# Tasks: Lift the migration quarantine

## 1. Guidance

- [x] 1.1 Prune `AGENTS.md`, including its quarantine section (separate
      commit, before this change).
- [x] 1.2 Remove quarantine and freeze language from `openspec/config.yaml`,
      `README.md`, `CONTRIBUTING.md`, `SUPPORT.md`, `docs/upstream-graft.md`,
      `.codecov.yml`, and the comments in
      `publish-images.yml` and `internal/model/doc.go` (**L1**).
- [x] 1.3 Update `.golangci.yml`'s `api/` block comment: condition met,
      removal tracked as `migrate-api` task 5.7 (**L5**).

## 2. Import edges

- [x] 2.1 Delete `presubmits.yml`'s `import-edges` job (**L2**).
- [x] 2.2 Remove the PR template's import-edge checkbox.
- [x] 2.3 Remove the isolation requirements from `migrate-api`'s `hosted-api`
      and `migrate-batch-pipeline`'s `batch-pipeline` spec deltas.

## 3. `migrate-api`

- [x] 3.1 Remove the quarantine requirement from the `hosted-api` spec delta.
- [x] 3.2 Mark **W10**'s constraints lifted in `proposal.md` and `design.md`,
      and the matching note in `migrate-batch-pipeline`'s `design.md`.

## 4. `x-google-backend`

- [x] 4.1 Remove the block from `api/openapi.yaml` and regenerate with the
      pinned go-swagger, in its own commit (**L4**).

## 5. Verification

- [x] 5.1 `go build ./...` and `golangci-lint` clean; `go test` for
      `internal/` and `cmd/` (the only Go change is a comment in
      `internal/model`); `actionlint` and `zizmor` clean on the changed
      workflows.
- [x] 5.2 `openspec validate --strict` for this change, `migrate-api`, and
      `migrate-batch-pipeline`.
- [x] 5.3 No remaining current-tense quarantine or freeze language outside
      historical records (**L3**).
