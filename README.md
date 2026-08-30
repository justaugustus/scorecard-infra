<!--
Copyright 2026 OpenSSF Scorecard Authors.
SPDX-License-Identifier: Apache-2.0
-->

# OpenSSF Scorecard Infrastructure

[![Presubmits](https://github.com/ossf/scorecard-infra/actions/workflows/presubmits.yml/badge.svg?branch=main)](https://github.com/ossf/scorecard-infra/actions/workflows/presubmits.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ossf/scorecard-infra/badge)](https://scorecard.dev/viewer/?uri=github.com/ossf/scorecard-infra)
[![Contributor-Covenant](https://img.shields.io/badge/Contributor%20Covenant-2.1-fbab2c.svg)](CODE_OF_CONDUCT.md)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](go.mod)

The infrastructure that runs [OpenSSF Scorecard](https://github.com/ossf/scorecard)
as a service — and the work to make it run anywhere.

> **Scorecard results are heuristic signals, not a verdict.** Nothing here asserts
> that a repository "is secure" or "is insecure"; every result declares its source,
> freshness, and completeness. A score of `-1` means *inconclusive*, not failing.

## Contents

- [About The Project](#about-the-project)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
- [Usage](#usage)
  - [Batch scanning pipeline](#batch-scanning-pipeline)
  - [Results API (api.scorecard.dev)](#results-api-apiscorecarddev)
  - [Results API server](#results-api-server)
  - [Provider-agnostic migration](#provider-agnostic-migration)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)
- [Contact](#contact)
- [Acknowledgements](#acknowledgements)

## About The Project

`ossf/scorecard` is the **engine**: checks, probes, scoring, output formats. This
repository is what surrounds it — the batch pipeline that scans 1M+ repositories
every week, the API that serves what it produces, and the design for moving that
stack off any single cloud provider.

| | Path | Role | State |
| --- | --- | --- | --- |
| **Batch scanning pipeline** | `cron/` | **Producer** — scans 1M+ repositories weekly and writes results to object storage | Production on GCP; migration to AWS in progress. Imported from `ossf/scorecard`. |
| **Results API** | `api/` | **Serving tier** — the service behind `api.scorecard.dev` | Production on AWS. Imported from `ossf/scorecard-webapp`. **This is what ships.** |
| **Provider-agnostic design** | `docs/research/` | **The plan** — a reference design for running Scorecard's hosted data services on any cloud or self-hosted target | Reference design; not yet an official artifact. |

These are one system's present and future, not three unrelated projects. The
pipeline is a *producer* writing Scorecard results to a bucket; the API is the
*consumer* reading them back out over HTTP. Both arrived here from elsewhere, so
that the whole hosted data path lives in one place — which is the precondition
for moving it.

They share a repository and **no code**. There are no import edges in any
direction, enforced by CI, and that is deliberate. Converging them is the point
of landing them together; it has not happened yet and should not happen
incidentally (designs **C11** and **W10**, in
[`migrate-batch-pipeline`](openspec/changes/migrate-batch-pipeline/design.md) and
[`migrate-api`](openspec/changes/migrate-api/design.md)).

**Where the effort is going.** Both trees are actively developed, but the work
is scoped to what supports the migration described in
[`scorecard#5208`](https://github.com/ossf/scorecard/issues/5208). For `cron/`,
that additionally means keeping the tree equivalent to what `ossf/scorecard`
builds until its cutover completes — that equivalence is the acceptance test and
the rollback path. Contributions outside that scope are welcome but will
generally wait.

## Getting Started

### Prerequisites

Common to everything here:

- **Go** matching [`go.mod`](go.mod) (1.25.x). If builds fail with a Go tool
  version mismatch across many stdlib packages, a stray `GOROOT` is the cause —
  prefix commands with `env -u GOROOT`.

Each component adds its own — an object store and SCM token, `ko` and `protoc`,
a cluster and its cloud dependencies. They are listed in that component's
README, linked from [Usage](#usage) below.

### Installation

```sh
git clone https://github.com/ossf/scorecard-infra
cd scorecard-infra
make build     # go build ./...
make help      # every target, grouped
```

Building an individual component:

| Component | Where |
| --- | --- |
| **Results API** — the service that ships | [`api/README.md`](api/README.md) |
| **Batch scanning pipeline** | [`cron/README.md`](cron/README.md) |

## Usage

### Batch scanning pipeline

`cron/` is the batch scanning pipeline behind the weekly public Scorecard scan of
1M+ repositories. It was imported from
[`ossf/scorecard`](https://github.com/ossf/scorecard) with its full commit history
— 466 commits dating to 2020 — because the operational history *is* the runbook:
quota workarounds, shard sizing, PubSub ack deadlines, and BigQuery schema
migrations exist only as commit messages.
[`cron/initial-graft.md`](cron/initial-graft.md) records the terms of that import,
including what could not come across and where to trace it.

**See [`cron/README.md`](cron/README.md)** for the pipeline components, the scan
inventories and how to add a repository to the weekly scan, and the build
targets.

### Results API (api.scorecard.dev)

The service behind `api.scorecard.dev` and `api.securityscorecards.dev`,
imported from [`ossf/scorecard-webapp`](https://github.com/ossf/scorecard-webapp)
with its full history. **This is the API that ships.** It is what every live
consumer already talks to: the project website's result viewer,
`img.shields.io`, the Scorecard GitHub Action's upload path, and `scorecard-mcp`.

The website that consumed it stays in `ossf/scorecard-webapp`, which remains its
home; only the Go API moved.

Endpoints, unchanged by the migration:

| Endpoint | Behavior |
| --- | --- |
| `GET /projects/{platform}/{org}/{repo}` | Published result; `?commit=` for a pinned one |
| `GET /projects/{platform}/{org}/{repo}/badge` | `302` to `img.shields.io` (a redirect — it renders nothing itself) |
| `POST /projects/{platform}/{org}/{repo}` | Publish a result, after verifying its Sigstore certificate, transparency-log entry, and originating workflow |

Found a bug in the API? [Open an issue](https://github.com/ossf/scorecard-infra/issues/new/choose)
in this repository. Bugs in the *viewer* at `scorecard.dev` belong in
[`ossf/scorecard-webapp`](https://github.com/ossf/scorecard-webapp/issues).

**See [`api/README.md`](api/README.md)** for the tree layout, build targets,
image publication, and the two counterintuitive things about this tree —
and [`deploy/api/README.md`](deploy/api/README.md) for the deployment itself.

### Results API server

`cmd/scorecard-api` and `internal/` hold a separate, provider-agnostic
implementation of the results contract. **It is not the deployment path, and
nothing in production uses it** — what ships is `api/`, above. It is documented
in [`cmd/scorecard-api/README.md`](cmd/scorecard-api/README.md).

### Provider-agnostic migration

Scorecard's hosted infrastructure is being migrated to a new home. The purpose of
the work in `docs/research/` is to make that migration **provider-agnostic** —
portable across any cloud or self-hosted target rather than tied to a single
provider — with the smallest reliable footprint.

- [`docs/research/data-infra.md`](docs/research/data-infra.md) — the reconciled
  reference design: object store, serving tier, interchange format, public
  dataset, and cost transparency. Written to be proposable to the Scorecard
  **Infrastructure Working Group**; it is not an official artifact yet, and
  nothing in it is committed.
- [`docs/research/infra-seed-0.md`](docs/research/infra-seed-0.md) and
  [`infra-seed-1.md`](docs/research/infra-seed-1.md) — the two research passes it
  reconciles (component-selection breadth; correctness/protocol critique and data
  model).

This is the arc that gives the two systems above a shared destination. Getting the
pipeline out of `ossf/scorecard` is the precondition for it, not the work itself.

## Roadmap

The infrastructure migration — what is moving, what has already moved, and what
it means for consumers — is announced and tracked in
[`ossf/scorecard#5208`](https://github.com/ossf/scorecard/issues/5208).

Backlog ownership across the Scorecard repositories, including which issues
belong here, is in
[`#77`](https://github.com/ossf/scorecard-infra/issues/77).

Per-change plans live in [`openspec/changes/`](openspec/changes/); `openspec
list` shows their live status.

## Contributing

Contributions are welcome. For detailed contributing guidelines — including how
to add a repository to the weekly scan, and the rules that apply to the `cron/`
tree — please see [CONTRIBUTING.md](CONTRIBUTING.md).

```sh
make build     # go build ./...
make test      # go test ./... -race
make lint      # golangci-lint run ./...   (config in .golangci.yml, aligned with ossf/scorecard)
make help      # every target, grouped — including the pipeline targets
```

An S3-compatible integration test runs when `SCORECARD_TEST_S3_URL` is set (e.g. a
local self-hosted S3-compatible store), and is skipped otherwise.

This repository is developed spec-first with
[OpenSpec](https://github.com/Fission-AI/OpenSpec); see [`AGENTS.md`](AGENTS.md)
for conventions and [`openspec/`](openspec/) for the specs and change proposals.

Participation is governed by our [Code of Conduct](CODE_OF_CONDUCT.md).

## License

Distributed under the Apache-2.0 License. See [LICENSE](LICENSE) for more
information.

Data served from Scorecard is licensed under
[CDLA Permissive 2.0](https://github.com/ossf/scorecard#scorecard-rest-api).

## Contact

- **Maintainers** — [MAINTAINERS.md](MAINTAINERS.md)
- **Questions and help** — [SUPPORT.md](SUPPORT.md), or
  [open an issue](https://github.com/ossf/scorecard-infra/issues/new/choose)
- **Security vulnerabilities** — [SECURITY.md](SECURITY.md). Please do **not**
  open a public issue; escalation goes to
  [scorecard-steering@lists.openssf.org](mailto:scorecard-steering@lists.openssf.org).

Project Link:
[https://github.com/ossf/scorecard-infra](https://github.com/ossf/scorecard-infra)

## Acknowledgements

- The batch scanning pipeline was imported from
  [`ossf/scorecard`](https://github.com/ossf/scorecard) with its full commit
  history. `git blame` attributes to its original authors, not to the merge —
  see [`cron/initial-graft.md`](cron/initial-graft.md).
- The results API was imported from
  [`ossf/scorecard-webapp`](https://github.com/ossf/scorecard-webapp) with its
  full history — see [`api/initial-graft.md`](api/initial-graft.md).
- This README was adapted from
  [https://github.com/bloomberg/oss-template](https://github.com/bloomberg/oss-template).
