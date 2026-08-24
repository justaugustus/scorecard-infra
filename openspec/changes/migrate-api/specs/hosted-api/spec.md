# hosted-api: Added Requirements

## ADDED Requirements

### Requirement: Results retrieval behavior is preserved by the migration

The imported API SHALL serve published Scorecard results for a project, by
platform, organization, and repository, and SHALL support retrieving the result
for a specific commit as well as the latest result. Its responses SHALL be
identical to those served before the migration.

#### Scenario: Latest result served

- **WHEN** a client requests a project's results without specifying a commit
- **THEN** the latest published result SHALL be returned

#### Scenario: Pinned result served

- **WHEN** a client requests a project's results for a specific commit
- **THEN** the result published for that commit SHALL be returned

#### Scenario: Missing result reported as not found

- **WHEN** a client requests results for a project that has none published
- **THEN** a not-found response SHALL be returned rather than an error

#### Scenario: Responses match the pre-migration service

- **WHEN** the migrated service and the pre-migration service are queried with
  the same request
- **THEN** status code, body, and cache headers SHALL match

### Requirement: Badge rendering behavior is preserved

The imported API SHALL serve a score badge for a project in each style it
supported before the migration.

#### Scenario: Badge served in each supported style

- **WHEN** a badge is requested in any style the pre-migration service accepted
- **THEN** the badge SHALL be rendered in that style

### Requirement: Signed result publication preserves its verification rules

The imported API SHALL continue to accept published results only when their
signing certificate, transparency-log entry, and originating workflow all pass
the verification rules that applied before the migration. No rule SHALL be
relaxed, removed, or reordered by the migration.

#### Scenario: Valid signed result accepted

- **WHEN** a result is published with a valid certificate, a verifiable
  transparency-log entry, and a compliant workflow
- **THEN** the result SHALL be accepted and stored

#### Scenario: Non-compliant workflow rejected

- **WHEN** a result is published from a workflow that violates a restriction the
  pre-migration service enforced
- **THEN** the result SHALL be rejected

#### Scenario: Result attributed to a commit not in the repository rejected

- **WHEN** a result is published claiming a commit that the named repository does
  not contain
- **THEN** the result SHALL be rejected

### Requirement: Cache and invalidation behavior is preserved

The imported API SHALL emit the same client and edge cache directives as before
the migration, and SHALL continue to invalidate cached responses at the CDN when
a new result is published.

#### Scenario: Cache directives unchanged

- **WHEN** any response is served
- **THEN** its client and edge cache directives SHALL match those served before
  the migration

#### Scenario: Publication invalidates the cached response

- **WHEN** a new result is published for a project
- **THEN** the cached response for that project SHALL be invalidated

### Requirement: The imported implementation is the one that is deployed

Where this repository contains more than one implementation of the published
results contract, the imported one SHALL be the implementation that is built,
released, and deployed. The pre-existing implementation SHALL remain in the
repository, buildable and tested, but off the deployment path.

#### Scenario: Deployment uses the imported implementation

- **WHEN** the results API is built and deployed from this repository
- **THEN** the imported implementation SHALL be the one deployed

#### Scenario: The superseded implementation is retained, not removed

- **WHEN** the imported implementation becomes the deployment path
- **THEN** the pre-existing implementation SHALL remain present and SHALL
  continue to build and pass its tests

#### Scenario: The status of each implementation is discoverable

- **WHEN** a contributor reads this repository's guidance or the superseded
  packages themselves
- **THEN** they SHALL find which implementation is deployed, and that the choice
  is revisitable

### Requirement: The imported API stays isolated from this repository's other systems

The imported API SHALL remain free of source dependencies on this repository's
own API server and batch pipeline, in both directions, until a subsequent change
reconciles them.

#### Scenario: No import edges in either direction

- **WHEN** the repository's package dependencies are inspected
- **THEN** the imported API SHALL neither import nor be imported by this
  repository's own API server or batch pipeline packages

#### Scenario: Isolation is enforced automatically

- **WHEN** a change introduces such a dependency
- **THEN** CI SHALL fail

### Requirement: The imported API's cloud coupling is quarantined, not remediated

The imported API's hardcoded object-store locations SHALL be preserved unchanged
while it is behavior-frozen, notwithstanding this repository's cloud-agnostic
rules, which govern its own code. The quarantine SHALL be documented where an
implementer will encounter it.

#### Scenario: Hardcoded storage locations retained during the freeze

- **WHEN** the imported API is modified during the migration
- **THEN** its object-store locations SHALL remain unchanged

#### Scenario: The exception is discoverable

- **WHEN** a contributor reads this repository's agent and contributor guidance
- **THEN** that guidance SHALL state that the imported tree is exempt and why
