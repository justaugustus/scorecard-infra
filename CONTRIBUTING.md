# Contributing to scorecard-infra

Thanks for your interest in contributing! This repository holds two systems with
different rules, so start by finding yours:

| I want to… | Go to |
| --- | --- |
| Add a repository to the weekly public scan | [Adding repositories to the weekly scan](#adding-repositories-to-the-weekly-scan) |
| Change the batch scanning pipeline (`cron/`) | [Working on the batch pipeline](#working-on-the-batch-pipeline) |
| Change the results API server | [Working on the API server](#working-on-the-api-server) |
| Report a security vulnerability | [SECURITY.md](SECURITY.md) — **not** a public issue |

Everything below the routing table applies to all of them.

## License and sign-off

- This project is licensed under **Apache-2.0**. By contributing you agree your
  contributions are licensed under the same terms.
- Every commit must be **signed off** under the
  [Developer Certificate of Origin](https://developercertificate.org/) (DCO):

  ```sh
  git commit -s -m "your message"
  ```

  The sign-off adds a `Signed-off-by:` trailer with your name and email.

## Adding repositories to the weekly scan

The inventories that drive the weekly public Scorecard scan live in
`cron/internal/data/`:

| File | Scope |
| --- | --- |
| `projects.csv` | GitHub — rows are `github.com/{org}/{repo}` |
| `gitlab-projects.csv` | GitLab — rows are full `https://gitlab.com/…` URLs |
| `gitlab-projects-releasetest.csv` | GitLab release testing |

> [!NOTE]
> These files previously lived in `ossf/scorecard`. If you followed a link or a
> bookmark there, this is the right place now.

To add a repository:

1. Append it to the relevant file. Each row is `repo,metadata`; leave `metadata`
   empty if you have none.
2. Run `make add-projects`. It sorts and de-duplicates both the GitHub and GitLab
   inventories in place, so your hand-edit ends up in canonical form.
3. Commit the normalized result.

CI enforces two things: that the inventories are valid (`make validate-projects`)
and that `make add-projects` is a **no-op** against them. A hand-edit the tooling
would have reformatted therefore fails the build — run the target rather than
matching the format by eye.

Adding a repository is an ordinary pull request. It does not need a spec.

## Working on the batch pipeline

`cron/` was imported from `ossf/scorecard` with full history and is **not**
ordinary code in this repository. Read
[`cron/initial-graft.md`](cron/initial-graft.md) first, then note two rules that
apply here and nowhere else:

- **The tree is behavior-frozen** until the production cutover completes. It must
  stay equivalent to what `ossf/scorecard` builds, because that equivalence is the
  migration's acceptance test *and* its rollback path. Refactors, restructuring
  into this repo's `internal/` + `cmd/` layout, and cleanups are deferred by
  design — not overlooked.
- **`cron/internal/format` owns a published contract.** It serializes a data model
  that lives upstream in the Scorecard engine, so a schema edit here without the
  corresponding engine change breaks the BigQuery/JSON contract silently.
  `schema_gen_test.go` is what catches it; do not skip or weaken it.

Do **not** wire `cron/` and the API server together. There are no import edges in
either direction, and that absence is deliberate — it is what keeps the migration
revertible. Convergence needs its own spec (design **C11**).

Pipeline-specific targets (`make help` lists them all):

```sh
make dockerbuild           # build all six container images
make ko-images             # the same six via ko
make validate-projects     # validate the scan inventories
make build-proto           # regenerate protobufs (requires protoc on PATH)
```

`ko` and `protoc` are expected on `PATH` rather than vendored; the Makefile
explains why and errors actionably when they are missing. `build-proto` is
deliberately explicit rather than a file rule — run it when you change a
`.proto`, and commit the generated output.

## Working on the API server

The server (`cmd/scorecard-api`, `internal/`) is an **incubator, not a permanent
fork**: its durable pieces are meant to graft into `ossf/scorecard-webapp` and
`ossf/scorecard`'s `scorecard serve`. Structure changes accordingly — keep
`store` and the `/projects` handlers thin and faithful to the webapp so they lift
out cleanly, and keep `scan` a thin adapter over `pkg/scorecard`. See
[`docs/upstream-graft.md`](docs/upstream-graft.md) for the per-component map.

Two conventions are easy to get wrong:

- **Cloud-agnostic is non-negotiable.** No hardcoded bucket URLs, no `gs://`
  constants, no BigQuery. Everything is configured by environment variable, and
  every blob driver is blank-imported so the backend is chosen purely by URL.
- **Configuration and feature flags are different mechanisms.** If the process
  needs it to boot or connect, it is configuration (`internal/config`). If you
  would change it without a redeploy — a kill-switch, a rollout, a mode shift —
  it is a feature flag (`internal/flags`).

## Development

Requires Go matching the version in [`go.mod`](go.mod).

```sh
make build     # go build ./...
make test      # go test ./... -race
make lint      # golangci-lint run ./...   (config in .golangci.yml)
make help      # every target, grouped
```

Please keep all three clean before opening a pull request. Workflows are also
linted with `actionlint` and `zizmor`.

If builds fail with a Go tool version mismatch across many stdlib packages, a
stray `GOROOT` is the cause — prefix commands with `env -u GOROOT`.

## Spec-driven workflow (OpenSpec)

This repository is developed spec-first using
[OpenSpec](https://github.com/Fission-AI/OpenSpec). The flow is: explore →
propose → design → specs → tasks → implement, keeping specs and code in sync.

- Canonical specs live in [`openspec/specs/`](openspec/specs/); active and
  archived changes in [`openspec/changes/`](openspec/changes/).
- Do **not** restructure behavior without updating the corresponding spec.
- Adding a repository to the scan inventories is the exception — it is data, not
  behavior, and needs no spec.

## Pull requests

- Work on a **feature branch**; do not commit directly to `main`.
- Keep each PR focused; prefer a single atomic commit per logical change with a
  descriptive body explaining the *why*.
- Fill out the pull request template, including how the change was tested.
- Update docs/specs when behavior changes.

## Project boundaries

- This is an open-source project: **do not add employer- or deployment-specific
  references** (object-store endpoints, org/repo scan lists beyond the public
  inventories, orchestration manifests, token sourcing). Those belong in a
  separate deployment repo.

## Reporting security issues

See [SECURITY.md](SECURITY.md). Please do not open public issues for
vulnerabilities.
