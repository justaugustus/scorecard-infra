# Support

## Getting Help

Thanks for using `scorecard-infra`! Here are some ways to get help.

### Documentation

Start with the [README](README.md) — it explains what is in this repository and
how the pieces relate. From there:

#### Both systems

- [CONTRIBUTING](CONTRIBUTING.md) — how to contribute, routed by what you're
  changing
- [AGENTS.md](AGENTS.md) — conventions, tooling, and the spec-driven workflow
- [openspec/](openspec/) — canonical specs and change proposals

#### Batch scanning pipeline (`cron/`)

- [cron/initial-graft.md](cron/initial-graft.md) — how the pipeline was imported
  from `ossf/scorecard`, and what to know before changing it
- [cron/k8s/README.md](cron/k8s/README.md) — applying manifests to the cluster
- [openspec/changes/migrate-batch-pipeline/](openspec/changes/migrate-batch-pipeline/)
  — the in-flight migration

#### Results API server

- [docs/acceptance.md](docs/acceptance.md) — end-to-end acceptance runbook
- [docs/upstream-graft.md](docs/upstream-graft.md) — where each component is meant
  to land upstream
- [docs/bootstrap.md](docs/bootstrap.md) — historical record of the v0 design brief

#### Provider-agnostic migration

- [docs/research/data-infra.md](docs/research/data-infra.md) — the reference
  design, plus the two research passes it reconciles

### Issues

If you've found a bug or have a feature request:

1. Search [existing
   issues](https://github.com/ossf/scorecard-infra/issues) first
2. If not found, [open a new
   issue](https://github.com/ossf/scorecard-infra/issues/new/choose)

Issues about Scorecard **checks, probes, scores, or output formats** belong in
[`ossf/scorecard`](https://github.com/ossf/scorecard/issues) — this repository
runs the engine, it does not define what the engine measures.

### Adding a repository to the weekly scan

This is the most common reason people arrive here, and it does not need an issue.
See
[Adding repositories to the weekly scan](CONTRIBUTING.md#adding-repositories-to-the-weekly-scan).
The inventories moved here from `ossf/scorecard`; if you followed a link there,
this is the right place.

### Security Issues

**Please do not report security vulnerabilities through public issues.**

See our [Security Policy](SECURITY.md) for responsible disclosure instructions.

## Response Times

This is an open source project. Response times may vary based on maintainer
availability. We appreciate your patience!

## Support Scope

The two systems in this repository are at different maturities:

| Component | Status | Support |
| --- | --- | --- |
| Batch scanning pipeline (`cron/`) | Production | Runs the weekly public scan. Behavior-frozen until its migration out of `ossf/scorecard` completes; see [cron/initial-graft.md](cron/initial-graft.md). |
| Results API server | Pre-release | Pre-1.0 and under active development; interfaces may change. |
| Provider-agnostic design (`docs/research/`) | Reference design | Not an official artifact; nothing in it is committed. |

Support tracks the `main` branch in both cases. There are no tagged releases yet.
