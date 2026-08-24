# api-contract: Added Requirements

## ADDED Requirements

### Requirement: The published API contract is hosted here and unchanged by the migration

This repository SHALL host the OpenAPI contract that defines the published
results API, and the migration SHALL NOT alter it. The contract is consumed
externally — by the project website, by client generators, and by the API
gateway — so a change to it is a change to a deployed service.

#### Scenario: Contract content preserved across the move

- **WHEN** the imported contract is compared with the pre-migration contract
- **THEN** they SHALL be identical

#### Scenario: Contract changes are out of scope for the migration

- **WHEN** a migration task would require editing the contract
- **THEN** the task SHALL be deferred rather than the contract edited

### Requirement: Server and client code is generated from the contract, reproducibly

The API's server scaffolding, client, and models SHALL be generated from the
contract rather than hand-maintained, and regeneration SHALL be reproducible: a
regeneration run on a clean tree SHALL produce no diff.

#### Scenario: Regeneration produces no diff

- **WHEN** generation is re-run against an unmodified tree
- **THEN** the working tree SHALL be unchanged

#### Scenario: Drift between contract and generated code is caught

- **WHEN** the contract is changed without regenerating, or generated code is
  hand-edited
- **THEN** CI SHALL fail

#### Scenario: The generator version is recorded

- **WHEN** the generated tree is produced
- **THEN** the generator version used SHALL be recorded, so the output can be
  reproduced rather than approximated

### Requirement: Hand-owned wiring inside the generated tree is protected

The handler wiring, cross-origin configuration, and response encoding that live
inside the generated tree but are excluded from generation SHALL be preserved
across regeneration.

#### Scenario: Regeneration does not revert hand-owned wiring

- **WHEN** the generated tree is regenerated
- **THEN** the hand-owned configuration file SHALL be unchanged, and CI SHALL
  fail if it is not

### Requirement: The contract's dual role as deployment configuration is documented

The contract also carries gateway deployment configuration, so editing it
redeploys a service. This coupling SHALL be documented rather than left for a
contributor to discover by deploying.

#### Scenario: Coupling is stated where the contract is described

- **WHEN** a contributor reads this repository's documentation of the contract
- **THEN** it SHALL state that the contract doubles as gateway deployment
  configuration
