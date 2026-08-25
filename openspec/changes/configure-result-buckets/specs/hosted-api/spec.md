# hosted-api: Storage locations become configuration

## MODIFIED Requirements

### Requirement: The imported API's cloud coupling is quarantined, not remediated

The imported API's remaining provider-specific coupling SHALL be preserved
unchanged while it is behavior-frozen, notwithstanding this repository's
cloud-agnostic rules, which govern its own code. The quarantine SHALL be
documented where an implementer will encounter it, and SHALL list only the items
still frozen, so that the list does not read as broader permission than it is.

Object-store **locations** are no longer part of this quarantine; they are
governed by the requirement below. The API's published contract file, which is
simultaneously a deployed gateway's configuration, remains frozen.

#### Scenario: Remaining coupling retained during the freeze

- **WHEN** the imported API is modified during the migration
- **THEN** its published contract file SHALL remain unchanged, including the
  gateway configuration embedded in it

#### Scenario: The exception is discoverable

- **WHEN** a contributor reads this repository's agent and contributor guidance
- **THEN** that guidance SHALL state that the imported tree is exempt and why

#### Scenario: A remediated item is removed from the list

- **WHEN** a quarantined item is remediated by its own change
- **THEN** that item SHALL be removed from the documented quarantine while the
  remaining items stay, rather than the list being removed wholesale

## ADDED Requirements

### Requirement: The results API's object-store locations are deployment configuration

The buckets the results API reads and writes SHALL be selectable at deployment
time rather than compiled in, and the service SHALL be able to address a bucket
that is not hosted by any single specific provider, so that relocating the
service is a configuration change rather than a code change.

#### Scenario: An unconfigured deployment is unchanged

- **WHEN** the service starts with no bucket configuration supplied
- **THEN** it SHALL read and write exactly the locations it used before they
  became configurable

#### Scenario: A configured deployment addresses a different store

- **WHEN** a bucket location is supplied at deployment time
- **THEN** the service SHALL use it for the corresponding path, and SHALL do so
  without requiring a code change to support that store's URL scheme

#### Scenario: Every addressable backend is reachable at run time

- **WHEN** the service is built
- **THEN** each object-store backend it is permitted to address SHALL be linked
  into the binary, so that an unsupported location fails at deployment rather
  than at the first request that uses it

#### Scenario: Absent configuration is not distinguished from empty

- **WHEN** a bucket location is supplied but empty
- **THEN** the service SHALL treat it as absent and use its default, rather than
  attempting to open an unusable location

#### Scenario: Published results remain retrievable

- **WHEN** the publish path stores a result and the retrieval path is
  subsequently asked for it
- **THEN** both SHALL address the same bucket, and the configuration SHALL NOT
  permit them to be pointed at different ones
