# pipeline-deployment: Added Requirements

## ADDED Requirements

### Requirement: Container images built from this repository

This repository SHALL build every container image the batch pipeline deploys —
the batch controller, batch worker, CII worker, BigQuery transfer, release
webhook, and GitHub token-pool server — and SHALL expose build targets for each.

#### Scenario: All images build

- **WHEN** the repository's image build runs
- **THEN** it SHALL produce all six pipeline images

#### Scenario: Build targets ported

- **WHEN** the pipeline's build system is ported
- **THEN** the build, container, `ko`, and protobuf-generation targets SHALL exist
  in this repository with paths adjusted for its root

### Requirement: Continuous integration covers the pipeline

The repository's CI SHALL build the pipeline images and run the pipeline's
inventory-validation jobs, and its automated dependency updates SHALL cover the
pipeline's container definitions.

#### Scenario: Image build job present

- **WHEN** CI runs on a change to the repository
- **THEN** the pipeline image build job SHALL run

#### Scenario: Dependency updates cover pipeline containers

- **WHEN** automated dependency update configuration is evaluated
- **THEN** it SHALL include every container definition directory in the pipeline
  tree

#### Scenario: Linting applies to the imported tree

- **WHEN** the repository's linters run
- **THEN** they SHALL apply to the imported pipeline tree, and lint failures SHALL
  be resolved by adjusting the configuration or the code rather than by exempting
  the tree

### Requirement: Staged validation before production cutover

Images built from this repository SHALL be published to a staging tag or registry
path — never to the tags the production Kubernetes manifests consume — and SHALL
be validated against their production-equivalent images before any cutover.

#### Scenario: Staging publication does not reach production

- **WHEN** this repository publishes pipeline images before cutover
- **THEN** they SHALL be published to a staging tag or registry path, and the tags
  consumed by the production manifests SHALL be unaffected

#### Scenario: Staging image validated before cutover

- **WHEN** cutover is proposed
- **THEN** a staging-built image SHALL have been compared against its production
  equivalent and confirmed behaviorally equivalent

### Requirement: Production cutover by build-trigger repointing

Production cutover SHALL be performed by repointing the build triggers in the
hosting cloud project from `ossf/scorecard` to this repository, and SHALL be
validated by a full end-to-end pipeline cycle.

#### Scenario: Cutover validated end to end

- **WHEN** the build triggers have been repointed
- **THEN** a full pipeline cycle SHALL be run from controller through message
  queue, worker, object storage, BigQuery transfer, and webhook, and its output
  SHALL be compared against the prior cycle

#### Scenario: Cutover blocked without trigger ownership

- **WHEN** the owner of build-trigger edit rights in the hosting cloud project has
  not been identified and scheduled
- **THEN** cutover SHALL NOT be attempted

#### Scenario: Prior trigger configuration captured

- **WHEN** the build triggers are repointed
- **THEN** their prior configuration SHALL have been recorded beforehand, since it
  is not under version control and the change cannot be reverted from the
  repository

### Requirement: Rollback is a configuration change until removal completes

Until the pipeline is removed from `ossf/scorecard`, rollback SHALL require only
repointing the build triggers back, with no code restoration.

#### Scenario: Rollback after a failed cutover

- **WHEN** the production cycle after cutover fails or diverges from the prior
  cycle
- **THEN** repointing the build triggers back to `ossf/scorecard` SHALL restore
  the prior production configuration without restoring any deleted code

#### Scenario: Removal gated on a clean production cycle

- **WHEN** removal from `ossf/scorecard` is proposed
- **THEN** production SHALL have run on images built from this repository for at
  least one full scan cycle without divergence

### Requirement: Pipeline wiring removed from ossf/scorecard

After cutover completes, `ossf/scorecard` SHALL retain no pipeline code or
pipeline-specific build, CI, dependency-update, or coverage wiring, and its
remaining build SHALL be clean.

#### Scenario: Wiring stripped

- **WHEN** the pipeline is removed from `ossf/scorecard`
- **THEN** its build targets, image build entries, inventory-validation jobs,
  dependency-update paths, and coverage-ignore entry SHALL be removed with it

#### Scenario: Upstream remains buildable

- **WHEN** `ossf/scorecard` is built, tested, and linted after removal
- **THEN** it SHALL pass with no remaining references to the removed pipeline

### Requirement: Guard against new inbound coupling during the migration

While the pipeline exists in both repositories, `ossf/scorecard` SHALL fail CI on
any file outside the pipeline tree that imports a pipeline package, and that
guard SHALL be removed when the pipeline is removed.

#### Scenario: New inbound import rejected

- **WHEN** a change to `ossf/scorecard` adds an import of a pipeline package from
  outside the pipeline tree
- **THEN** CI SHALL fail

#### Scenario: Guard removed with the tree

- **WHEN** the pipeline is removed from `ossf/scorecard`
- **THEN** the guard SHALL be removed in the same change
