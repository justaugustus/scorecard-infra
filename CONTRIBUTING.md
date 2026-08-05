# Contributing to scorecard-api

Thanks for your interest in contributing! This document covers the essentials.

## License and sign-off

- This project is licensed under **Apache-2.0**. By contributing you agree your
  contributions are licensed under the same terms.
- Every commit must be **signed off** under the
  [Developer Certificate of Origin](https://developercertificate.org/) (DCO):

  ```sh
  git commit -s -m "your message"
  ```

  The sign-off adds a `Signed-off-by:` trailer with your name and email.

## Development

Requires Go matching the version in [`go.mod`](go.mod).

```sh
go build ./...                 # build everything
go test ./... -race            # run tests with the race detector
golangci-lint run ./...        # lint (config in .golangci.yml)
```

Please keep `go build`, `go test`, and `golangci-lint` all clean before opening a
pull request.

## Spec-driven workflow (OpenSpec)

This repository is developed spec-first using [OpenSpec](https://github.com/Fission-AI/OpenSpec).
The flow is: explore → propose → design → specs → tasks → implement, keeping specs
and code in sync.

- Active and archived changes live under [`openspec/`](openspec/).
- Do **not** restructure behavior without updating the corresponding spec.
- New contributors should start with [`docs/bootstrap.md`](docs/bootstrap.md).

## Pull requests

- Work on a **feature branch**; do not commit directly to `main`.
- Keep each PR focused; prefer a single atomic commit per logical change with a
  descriptive body explaining the *why*.
- Fill out the pull request template, including how the change was tested.
- Update docs/specs when behavior changes.

## Project boundaries

- This is an open-source project: **do not add employer- or deployment-specific
  references** (object-store endpoints, org/repo scan lists, orchestration
  manifests, token sourcing). Those belong in a separate deployment repo that
  feeds this server.

## Reporting security issues

See [SECURITY.md](SECURITY.md). Please do not open public issues for
vulnerabilities.
