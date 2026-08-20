# project-inventory: Added Requirements

## ADDED Requirements

### Requirement: Scan inventory hosted in this repository

This repository SHALL host the batch scan inventories that enumerate the
repositories scanned by the pipeline, including the GitHub and GitLab project
lists, and they SHALL remain a community contribution surface.

#### Scenario: Inventories present and contributable

- **WHEN** the inventories are imported
- **THEN** they SHALL be present in this repository and open to community
  contribution

### Requirement: Inventory contributions are validated automatically

The repository's CI SHALL validate inventory contributions and SHALL provide the
automation that normalizes additions, matching the checks that ran in
`ossf/scorecard` before the migration.

#### Scenario: Malformed inventory entry rejected

- **WHEN** a change adds a malformed or improperly ordered inventory entry
- **THEN** CI SHALL fail

#### Scenario: Addition automation available

- **WHEN** a contributor adds a repository to an inventory
- **THEN** the repository SHALL provide the automation that formats and validates
  the addition

### Requirement: The relocated contribution path is discoverable

Because the inventory's previous location is externally linked and widely
bookmarked, `ossf/scorecard` SHALL retain a redirect that points contributors to
the new location rather than returning nothing.

#### Scenario: Bookmarked path resolves to instructions

- **WHEN** a contributor navigates to the inventory's former location in
  `ossf/scorecard`
- **THEN** they SHALL find a pointer to the new location rather than a missing
  path

#### Scenario: Contribution documentation updated

- **WHEN** the pipeline is removed from `ossf/scorecard`
- **THEN** its contributing documentation SHALL be rewritten to reference this
  repository, with explicit URLs for each inventory file

#### Scenario: Misdirected contributions are redirected, not closed

- **WHEN** an inventory contribution is opened against `ossf/scorecard` after the
  migration
- **THEN** the repository SHALL surface guidance directing the contributor to the
  new location

### Requirement: External importers considered before removal

Before the pipeline is removed from `ossf/scorecard`, its publicly importable
packages SHALL be checked for known external consumers, and removal SHALL be
adjusted if any exist.

#### Scenario: No known consumers

- **WHEN** no external consumer of the pipeline's public packages is identified
- **THEN** those packages MAY be removed from `ossf/scorecard` without a
  deprecation period

#### Scenario: Known consumer identified

- **WHEN** an external consumer of a publicly importable pipeline package is
  identified
- **THEN** removal SHALL provide a deprecation window instead of an immediate
  delete
