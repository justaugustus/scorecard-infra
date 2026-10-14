# AGENTS.md

Read [`README.md`](README.md) first. This file holds only what the README
and the code don't tell you.

## Before you change anything

- **`api/` is the results API that ships.** `internal/httpapi` and
  `cmd/scorecard-api` implement the same contract but are off the
  deployment path, so don't add API surface there expecting it to ship.
  See [`docs/upstream-graft.md`](docs/upstream-graft.md).
- **New work starts as an OpenSpec change** under `openspec/changes/`. Check
  the active changes' `tasks.md` first (`openspec list`).
- **Scorecard results are signals, not a verdict.** Never assert that a
  repository "is secure" or "is insecure"; a score of `-1` is inconclusive,
  not failing. This applies to any code that formats or reports a score.

## Gotchas

- **`api/app/generated/restapi/configure_scorecard.go` is hand-owned**
  despite living in the generated tree. It holds the handler wiring, CORS,
  and the JSON producer, and is excluded from `SWAGGER_GEN`. A regeneration
  that overwrites it silently reverts routing.
- **`api/`'s end-to-end tests reach the network.** They call
  `api.github.com` (unauthenticated, they hit the 60/hour limit and flake)
  and live Rekor. They *skip* rather than fail when Rekor's search-by-hash
  index is unreachable, so check the skip count before trusting a local run.
- **`cron/internal/format` serializes a data model that lives upstream** in
  `ossf/scorecard`. A schema edit without the matching engine change breaks
  the contract silently; `schema_gen_test.go` is what catches it.
- **Stock actionlint rejects newer workflow keys** such as `cache-mode`,
  which `.github/workflows/scorecard.yml` uses. The API validates workflows
  with the `actionlint.kjanat.dev` fork pinned in `go.mod`; lint with that.
- **The API binary is still named `scorecard-webapp`.** Renaming it is its
  own change, not a drive-by.

## Build, test, lint

```sh
make build && make test && make lint   # make help lists every target
```

All three must be clean. Workflows are also checked with `zizmor` (and
actionlint, per the gotcha above).

## Rules

- **Cloud-agnostic.** No hardcoded bucket URLs; configuration comes from the
  environment. Blank-import every `gocloud.dev/blob` driver and let
  credentials resolve through each backend's default chain. No BigQuery in
  new code. Local development uses `fileblob`; tests use `memblob`.
- **Configuration vs. feature flags** (design **FF3**). If the process needs
  it to boot or connect, it is configuration: env-driven, validated at
  startup, in `internal/config`. If you would change it without a redeploy,
  it is a flag: read through `internal/flags` with a safe in-code default.

## Commits and PRs

- **DCO sign-off** on every commit (`git commit -s`). It is the human's
  certification; an AI agent never adds its own `Signed-off-by:`.
- **AI attribution** uses the kernel's
  [coding-assistants](https://docs.kernel.org/process/coding-assistants.html)
  trailer, `Assisted-by: LLM`, not `Co-Authored-By:`.
- **One atomic commit per logical change**, with a body explaining *why*.
  Reference design decisions and OpenSpec tasks.
- **Feature branches only**; never commit to `main`. Don't open PRs unless
  asked.
- **No employer or internal references** in files or commits.
