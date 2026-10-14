# imported-trees Specification

## Purpose
Defines how the trees imported from other repositories, `api/` and `cron/`, are
maintained now that their migrations are complete: under the same rules as the
rest of the repository, with no CI-enforced package boundary.
## Requirements
### Requirement: Imported trees follow the repository's rules

`api/` and `cron/` SHALL be held to the same rules as the rest of this
repository. No guidance SHALL exempt them as frozen or quarantined.

#### Scenario: A rule violation is found in an imported tree

- **WHEN** a contributor finds code in `api/` or `cron/` that violates a
  repository rule
- **THEN** it SHALL be fixed like any other code, by its own change

#### Scenario: Guidance is read

- **WHEN** a contributor reads this repository's agent, contributor, or
  OpenSpec guidance
- **THEN** it SHALL NOT describe `api/` or `cron/` as frozen or quarantined

### Requirement: Package boundaries are not enforced by CI

CI SHALL NOT fail a change because it adds an import between `api/`, `cron/`,
`internal/`, and `cmd/`. Converging the serving-tier implementations SHALL
still be done through its own change.

#### Scenario: A change adds an import across trees

- **WHEN** a change adds an import between two of these trees
- **THEN** CI SHALL NOT fail on that basis

