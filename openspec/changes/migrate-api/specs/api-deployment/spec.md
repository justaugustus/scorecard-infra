# api-deployment: Added Requirements

## ADDED Requirements

### Requirement: The API's container image is built in this repository

This repository SHALL build the API's container image, and its build SHALL run in
CI on every change so that a broken image is caught before deployment rather than
at deploy time.

#### Scenario: Image builds in CI

- **WHEN** a change touching the imported API is proposed
- **THEN** CI SHALL build its image and SHALL fail if the build fails

#### Scenario: Paths internal to build files are verified by building

- **WHEN** the imported tree is relocated
- **THEN** an image build SHALL be run to catch path references that no compiler
  checks

### Requirement: Releases are identified per component and by immutable artifact

Because this repository hosts more than one independently deployable system, a
release SHALL identify which component it releases rather than claiming the
repository as a whole. Deployments SHALL be able to name the exact artifact they
run, so that a rollback targets a known build rather than a moving reference.

#### Scenario: Release identifies its component

- **WHEN** a release of the API is tagged
- **THEN** the tag SHALL identify the API specifically, and SHALL NOT be
  ambiguous with a repository-wide or another component's release

#### Scenario: A deployed artifact can be named exactly

- **WHEN** a deployment is made or rolled back
- **THEN** the artifact SHALL be identifiable by an immutable reference rather
  than only by a mutable tag

### Requirement: Cutover is staged and verified against production before traffic moves

The migrated service SHALL be deployed to a non-production destination and
compared against the running production service before any traffic is shifted.
The comparison SHALL cover status codes, response bodies, and cache directives.

#### Scenario: Staging artifact is not published to the production tag

- **WHEN** the image is built for cutover verification
- **THEN** it SHALL be published to a staging destination that the production
  service does not consume

#### Scenario: Response comparison gates the traffic shift

- **WHEN** the staged deployment's responses differ from production's for any
  request in the agreed comparison set
- **THEN** traffic SHALL NOT be shifted until the difference is explained or
  resolved

#### Scenario: The publish path is exercised end to end

- **WHEN** traffic has been shifted
- **THEN** a complete result publication SHALL be confirmed before the hold
  period is considered started

### Requirement: Rollback is a traffic shift, not a code restoration

Until the code is removed upstream, rollback SHALL consist of shifting traffic
back to the previously captured deployment. The pre-cutover configuration of
every externally managed system SHALL be captured before it is changed.

#### Scenario: Prior configuration captured before change

- **WHEN** an externally managed system is about to be repointed
- **THEN** its current configuration SHALL be recorded first

#### Scenario: Rollback requires no code change

- **WHEN** a rollback is performed before upstream removal
- **THEN** it SHALL require no code restoration in either repository

### Requirement: Externally managed systems are repointed as part of cutover

Every system that names the source repository but is configured outside version
control SHALL be identified before cutover and repointed as part of it,
including the image build trigger, the API gateway configuration, the domain
mapping, and the fuzzing project.

#### Scenario: Repointing inventory is complete before cutover begins

- **WHEN** cutover begins
- **THEN** every externally managed system that names the source repository
  SHALL be enumerated with an owner

#### Scenario: Fuzzing coverage continues after the move

- **WHEN** cutover completes
- **THEN** continuous fuzzing SHALL run against this repository's copy of the
  API, not the pre-migration source

### Requirement: Upstream removal happens only after a clean hold period

The API SHALL NOT be removed from its source repository until the migrated
deployment has served production traffic without regression for the agreed hold
period.

#### Scenario: Removal blocked during the hold

- **WHEN** the hold period has not elapsed or a regression is open
- **THEN** upstream removal SHALL NOT proceed

#### Scenario: Source repository remains coherent after removal

- **WHEN** the API is removed upstream
- **THEN** the source repository SHALL remain a working website repository, with
  its documentation pointing to the API's new location
