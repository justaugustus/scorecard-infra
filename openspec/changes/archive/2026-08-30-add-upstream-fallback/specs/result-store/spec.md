# result-store: Added Requirements

## ADDED Requirements

### Requirement: Result origin tag

The store SHALL persist and retrieve an origin tag alongside each result —
distinguishing a locally-scanned result from one obtained upstream — without
altering the canonical JSON2 body. A result stored without an origin tag SHALL be
treated as locally scanned.

#### Scenario: Origin recorded on write

- **WHEN** a result is written with a given origin (locally scanned or upstream)
- **THEN** the store SHALL persist that origin as metadata separate from the JSON2
  body

#### Scenario: Origin returned on read

- **WHEN** a stored result is read
- **THEN** the store SHALL report its origin so the orchestrator can apply
  source-aware freshness and report the true source

#### Scenario: Legacy untagged result

- **WHEN** a stored result has no origin metadata
- **THEN** the store SHALL report it as locally scanned
