# release-versioning: One release line for the repository

## ADDED Requirements

### Requirement: Releases are versioned repository-wide

A release SHALL be tagged `vX.Y.Z` under Semantic Versioning and SHALL name a
release of the whole repository. New releases SHALL NOT use component-scoped
tags. The major version SHALL remain 0 until a separate decision moves it.

#### Scenario: A release is tagged

- **WHEN** a release is cut
- **THEN** its tag SHALL be a `v`-prefixed SemVer version with major version 0,
  and SHALL NOT carry a component prefix such as `api/` or `cron/`

#### Scenario: A release is withdrawn

- **WHEN** a published release must be withdrawn
- **THEN** it SHALL be retracted rather than deleted or re-pointed, because the
  tag is a permanent Go module version

### Requirement: A release publishes every image

Pushing a release tag SHALL publish every container image this repository
builds, from the tagged commit. Each system SHALL still deploy independently, by
digest.

#### Scenario: A release tag is pushed

- **WHEN** a `vX.Y.Z` tag is pushed
- **THEN** every image SHALL be published from that commit with the version
  `X.Y.Z` as a tag, alongside its commit SHA tag

#### Scenario: A pre-release tag is pushed

- **WHEN** a tag with a SemVer pre-release suffix, such as `v0.2.0-rc.1`, is
  pushed
- **THEN** every image SHALL be published with that version as a tag, and
  `latest` SHALL NOT be moved

### Requirement: Images record the engine version they run

Every published image SHALL identify the release it belongs to and the
Scorecard engine version and commit it was built against. What a deployment
runs SHALL be determinable from the artifact alone.

#### Scenario: An image is inspected

- **WHEN** a published image's labels are read
- **THEN** they SHALL include `org.opencontainers.image.version`,
  `dev.scorecard.engine.version`, and `dev.scorecard.engine.commit`

#### Scenario: The engine commit cannot be resolved

- **WHEN** the engine version cannot be resolved to a commit
- **THEN** publishing SHALL fail rather than publish an image with an empty or
  guessed engine commit
