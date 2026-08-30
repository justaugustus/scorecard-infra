# infrastructure-as-code Specification

## Purpose
Defines how cloud infrastructure for the hosted services is declared and
managed: reproducible from version control rather than console changes,
captured before it's changed, backed by shared and locked state, free of
secret values, pinned to a known toolchain version, and careful not to take
ownership of pre-existing data stores it didn't create.
## Requirements
### Requirement: Hosted infrastructure is reproducible from version control

Every cloud resource the hosted services depend on SHALL be declared in this
repository and creatable from that declaration alone. A resource that exists only
because someone created it in a console is not reproducible, cannot be reviewed,
and stops being knowable when account access lapses.

#### Scenario: A resource is created

- **WHEN** a cloud resource is required by the hosted services
- **THEN** it SHALL be declared in version control before it is created, and
  SHALL NOT be created by console or ad-hoc CLI action

#### Scenario: Resources created outside the declaration are identifiable

- **WHEN** infrastructure is provisioned
- **THEN** every declared resource SHALL carry a common tag set, so that a
  resource lacking it is identifiable as drift

### Requirement: The existing environment is captured before it is changed

Provisioning SHALL be preceded by a recorded capture of what the target account
already contains. Assumed topology has been wrong every time it has been checked
in this project, and a provisioning run that duplicates an existing resource is
harder to unwind than one that adopts it.

#### Scenario: Discovery precedes the first apply

- **WHEN** infrastructure is provisioned into an account for the first time
- **THEN** a capture of that account's existing resources SHALL have been run and
  reviewed first

#### Scenario: Capture degrades usefully

- **WHEN** a capture section fails on a permission or a missing resource
- **THEN** the failure SHALL be recorded and the run SHALL continue, because a
  partial capture is useful and an aborted one is not

#### Scenario: Absent credentials fail fast

- **WHEN** a capture runs without usable credentials
- **THEN** it SHALL report that once and stop, rather than repeating the same
  authentication error for every section

### Requirement: State is shared, locked, versioned, and separated per environment

Infrastructure state SHALL be stored remotely with locking enabled, versioning
enabled, and a distinct location per environment. Unlocked state permits
concurrent applies to corrupt it; unversioned state has no recovery path; shared
state across environments permits an apply intended for one to land on another.

#### Scenario: Concurrent applies are refused

- **WHEN** two applies against the same state run concurrently
- **THEN** the second SHALL be refused rather than proceeding

#### Scenario: Locking is verified rather than assumed

- **WHEN** a state backend is configured
- **THEN** its locking SHALL be demonstrated by a concurrent-apply test, because
  a backend that silently fails to lock is indistinguishable from one that works

#### Scenario: Environments cannot be confused

- **WHEN** an environment is targeted
- **THEN** the target SHALL be textual and reviewable in the configuration, and
  environments SHALL NOT share a state location

### Requirement: Secret values never enter infrastructure state or the repository

Infrastructure code SHALL create secret containers and grant access to them, and
SHALL NOT carry secret values. A value passed to a provisioning tool is a value
written to its state, which would make the state store a credential store with
weaker controls than the service built for the purpose.

#### Scenario: A secret is provisioned

- **WHEN** a secret is required by a workload
- **THEN** the infrastructure SHALL create the secret resource and the policy
  granting access, and the value SHALL be loaded out-of-band

#### Scenario: Out-of-band values are not reverted

- **WHEN** infrastructure is re-applied after a value has been loaded
- **THEN** it SHALL NOT overwrite or clear that value

### Requirement: The toolchain version required is declared, not discovered

Infrastructure code SHALL declare the minimum tool version it depends on. A
version-dependent feature that is absent produces a failure at the point of use,
far from its cause.

#### Scenario: The toolchain is too old

- **WHEN** infrastructure is planned with a tool version below the declared
  minimum
- **THEN** it SHALL fail with a version error naming the requirement

### Requirement: Pre-existing stores holding data are referenced, not managed

Infrastructure code SHALL reference an existing data store it did not create,
rather than declaring it as a creatable resource. A tool that can create a
bucket can destroy it, and it cannot distinguish "this should not exist" from
"someone deleted the block that declared it" — so managing a store that already
holds data puts that data one refactor away from deletion.

#### Scenario: A workload needs an existing store

- **WHEN** a workload requires a data store that already exists
- **THEN** the infrastructure SHALL reference it and grant access, and SHALL NOT
  declare it as a resource it could create or destroy

#### Scenario: Configuration of an existing store needs changing

- **WHEN** an existing store's own configuration is found wanting
- **THEN** that change SHALL be made as its own decision, and SHALL NOT arrive
  as a side effect of provisioning something that consumes the store

