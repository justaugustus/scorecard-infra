<!--
Copyright 2026 OpenSSF Scorecard Authors.
SPDX-License-Identifier: Apache-2.0
-->

# Batch scanning pipeline

`cron/` is the batch scanning pipeline behind the weekly public Scorecard scan of
1M+ repositories. It was imported from
[`ossf/scorecard`](https://github.com/ossf/scorecard) with its full commit history
— 466 commits dating to 2020 — because the operational history *is* the runbook:
quota workarounds, shard sizing, PubSub ack deadlines, and BigQuery schema
migrations exist only as commit messages.
[`initial-graft.md`](initial-graft.md) records the terms of that import,
including what could not come across and where to trace it.

Read it before working in this tree. Two rules apply here and nowhere else in
the repository:

- **Keep the tree equivalent to what `ossf/scorecard` builds** until the
  production cutover to AWS completes. That equivalence is the cutover's
  acceptance test *and* its rollback path, so refactors, restructuring into this
  repository's `internal/` + `cmd/` layout, and cleanups are deferred by design —
  not overlooked.
- **`cron/internal/format` serializes a data model that lives upstream**, so a
  schema edit here without the corresponding engine change breaks the published
  contract silently (`schema_gen_test.go` is what catches it).

## Pipeline components

| Component | Path | Image |
| --- | --- | --- |
| PubSub batch controller | `cron/internal/controller/` | `scorecard-batch-controller` |
| Batch scan worker | `cron/internal/worker/` | `scorecard-batch-worker` |
| CII best-practices worker | `cron/internal/cii/` | `scorecard-cii-worker` |
| BigQuery transfer | `cron/internal/bq/` | `scorecard-bq-transfer` |
| Release webhook | `cron/internal/webhook/` | `scorecard-webhook-releasetest` |
| GitHub token-pool server | `cron/internal/githubserver/` | `scorecard-github-server` |

Deployment manifests are in [`k8s/`](k8s/README.md); image build configs in
`cron/cloudbuild/`. `cron/internal/format/` owns the published BigQuery and JSON
schema contract, verified against the Scorecard engine's data model by
`schema_gen_test.go`.

## Scan inventories

The repositories scanned each week are listed in `cron/internal/data/`:

| File | Scope |
| --- | --- |
| `projects.csv` | GitHub |
| `gitlab-projects.csv` | GitLab |
| `gitlab-projects-releasetest.csv` | GitLab release testing |

To add a repository, edit the relevant file and run `make add-projects`, which
normalizes the result. CI enforces both that the inventories are valid and that
`add-projects` is a no-op against them, so a hand-edit the tooling would not
produce fails the build. See [`CONTRIBUTING.md`](../CONTRIBUTING.md#adding-repositories-to-the-weekly-scan)
for the full contribution path.

> [!NOTE]
> These inventories previously lived in `ossf/scorecard`. If you followed a link
> there, this is the right place now.

## Prerequisites

To build the images or regenerate the protobufs, **`ko`** and **`protoc`** must
be on `PATH`. They are expected rather than vendored; the Makefile explains why
and fails with actionable errors when they are missing.

Running the pipeline itself additionally requires its GCP dependencies (PubSub,
GCS, BigQuery) and the cluster described in [`k8s/README.md`](k8s/README.md).

## Pipeline targets

Run from the repository root:

```sh
make help                  # every target, grouped
make build-controller      # one pipeline binary (also build-worker, …)
make dockerbuild           # build all six images
make ko-images             # the same six via ko
make validate-projects     # validate the scan inventories
make build-proto           # regenerate protobufs (requires protoc on PATH)
```

## Also in this tree

- [`k8s/README.md`](k8s/README.md) — the cluster the pipeline runs on.
- [`data/README.md`](data/README.md) — the scan inventory data files.
- [`internal/emulator/README.md`](internal/emulator/README.md) — local PubSub
  emulation for development.
