# batch-pipeline: Added Requirements

## ADDED Requirements

### Requirement: Batch scanning components hosted in this repository

The repository SHALL host the batch scanning pipeline components — the PubSub
batch controller, the batch worker, the CII worker, the BigQuery transfer job,
the release webhook, and the GitHub token-pool RPC server — as a self-contained
tree, together with their Kubernetes manifests and Cloud Build configurations.

#### Scenario: Component set present

- **WHEN** the imported tree is inspected after the migration
- **THEN** it SHALL contain the controller, batch worker, CII worker, BigQuery
  transfer, release webhook, and GitHub token-pool server components, their
  Kubernetes manifests, and their Cloud Build configurations

#### Scenario: Token-pool server relocated without its parent package

- **WHEN** the GitHub token-pool RPC server is imported
- **THEN** only the server component SHALL move, and the token accessor,
  round-robin, and RPC client package it depends on SHALL remain in
  `ossf/scorecard`, unchanged and still consumed by the Scorecard round-tripper

### Requirement: Runtime behavior preserved across the migration

The migration SHALL NOT change pipeline runtime behavior, scan cadence, sharding,
or output schema, and SHALL NOT change any Scorecard check, probe, score, or
output format.

#### Scenario: Image behavioral equivalence

- **WHEN** an image built from this repository is compared against the equivalent
  image built from `ossf/scorecard`
- **THEN** the two SHALL be behaviorally equivalent, and any observed difference
  SHALL block the production cutover

#### Scenario: Pipeline output equivalence

- **WHEN** a full pipeline cycle completes using images built from this repository
- **THEN** its output row counts and schema SHALL match those of the prior cycle
  produced from `ossf/scorecard`-built images

### Requirement: Dependency on the published Scorecard engine module

The pipeline SHALL consume the Scorecard scan engine only through exported
packages of the published `github.com/ossf/scorecard/v5` module, and SHALL NOT
require any new API surface to be exported from `ossf/scorecard`.

#### Scenario: Engine consumed through the published module

- **WHEN** the pipeline builds in this repository
- **THEN** every Scorecard engine package it imports SHALL be an exported,
  non-`internal` package resolved from the declared module requirement

#### Scenario: Engine version is explicit

- **WHEN** the pipeline is built for production
- **THEN** the Scorecard engine version it builds against SHALL be explicitly
  declared in this repository's module requirements rather than implied by an
  in-tree copy of the engine

#### Scenario: Engine updates are surfaced deliberately

- **WHEN** a newer Scorecard engine release becomes available
- **THEN** the update SHALL reach the pipeline through an explicit, automated
  dependency bump, and a resulting build or test failure SHALL fail that bump
  rather than be reported as a warning

### Requirement: Pre-release engine breakage is detected on a schedule

Because the pipeline no longer builds against the engine in-tree, a scheduled job
SHALL build and test it against the engine's development branch, so breakage is
detected before it reaches a pinned release.

#### Scenario: Unreleased engine change breaks the pipeline

- **WHEN** the scheduled job builds and tests the pipeline against the engine's
  development branch and the build or tests fail
- **THEN** the failure SHALL be reported to a channel the maintainers monitor

#### Scenario: The canary does not gate unrelated work

- **WHEN** the scheduled job is failing because of an upstream change
- **THEN** it SHALL NOT block pull requests in this repository, which build
  against the pinned release

#### Scenario: Production builds remain reproducible

- **WHEN** a production image is built
- **THEN** it SHALL build against the explicitly pinned engine release, not
  against the development branch the scheduled job tracks

### Requirement: Pipeline code isolated from the API server capabilities

The imported pipeline SHALL remain self-contained: it SHALL NOT import this
repository's API server packages, and those packages SHALL NOT import the
pipeline.

#### Scenario: No new coupling introduced by the migration

- **WHEN** the migration completes
- **THEN** no import edge SHALL exist in either direction between the imported
  pipeline tree and the API server's store, cache, scan, flags, fallback, or HTTP
  packages

### Requirement: Published schema contract ownership

The pipeline SHALL own the published BigQuery and JSON schema contract it
serializes, and SHALL verify at test time that those schemas match the Go data
model of the Scorecard engine version it builds against.

#### Scenario: Schema verified against the engine data model

- **WHEN** the pipeline's test suite runs
- **THEN** it SHALL verify the published schemas against the engine's data model
  and SHALL fail on a mismatch

#### Scenario: Drift surfaced at dependency-bump time

- **WHEN** an engine dependency bump introduces a data model change that the
  published schemas do not reflect
- **THEN** the schema verification SHALL fail the bump

#### Scenario: Drift surfaced on the engine's cadence

- **WHEN** the scheduled job builds the pipeline against the engine's development
  branch
- **THEN** it SHALL run the schema verification against that branch's data model,
  so drift is detected before the corresponding release is pinned

### Requirement: Protobuf package paths are correct for this module

Generated protobuf definitions in the pipeline SHALL declare a `go_package` that
matches this repository's module path, and the generated Go sources SHALL be
regenerated from those definitions.

#### Scenario: Stale package path corrected

- **WHEN** the pipeline's protobuf definitions are imported
- **THEN** their `go_package` declarations SHALL be corrected from the stale
  `ossf/scorecard` path to this module's path, and the generated sources SHALL be
  regenerated to match
