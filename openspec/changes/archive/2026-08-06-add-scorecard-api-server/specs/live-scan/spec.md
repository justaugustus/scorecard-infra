# live-scan: Added Requirements

## ADDED Requirements

### Requirement: On-demand scanning via the Scorecard library

The scan engine SHALL generate a repository's Scorecard result in-process using the
Scorecard library and SHALL return it as canonical JSON2.

#### Scenario: Scan a repository

- **WHEN** the orchestrator requests a scan for a repository reference (and optional commit)
- **THEN** the engine SHALL run Scorecard against that repository and return a JSON2 result

#### Scenario: Result written back to the store

- **WHEN** a scan completes successfully
- **THEN** the engine SHALL write the result to the store under the appropriate key

### Requirement: Clients created once and reused

The engine SHALL create its repository and auxiliary clients once and reuse them
across scans.

#### Scenario: Reused clients

- **WHEN** multiple scans run over the engine's lifetime
- **THEN** they SHALL share the initialized clients rather than recreating them per scan

### Requirement: Token and rate management

The engine SHALL manage source-control credentials through a token pool and SHALL
apply per-host rate limiting with backoff and retry.

#### Scenario: Rate limit encountered

- **WHEN** a source-control API rate limit is hit during a scan
- **THEN** the engine SHALL back off and retry rather than failing immediately

#### Scenario: Concurrent scans use distinct tokens

- **WHEN** multiple scans run concurrently
- **THEN** the engine SHALL draw credentials from the token pool rather than sharing a single token unsafely

### Requirement: Skip and failure handling

The engine SHALL distinguish a skippable condition (an unreachable or blocked
repository) from a fatal error.

#### Scenario: Repository unreachable

- **WHEN** a repository cannot be reached or is blocked
- **THEN** the engine SHALL report the scan as skipped rather than as a fatal error

#### Scenario: Fatal scan error

- **WHEN** a scan fails for a non-skippable reason
- **THEN** the engine SHALL surface the error to the orchestrator without writing a partial result

### Requirement: Declared coverage

The engine SHALL declare its coverage — that it runs the full check set and can scan
any repository its credentials can access, including private repositories — for the
server's capabilities reporting.

#### Scenario: Coverage advertised

- **WHEN** the server reports its capabilities
- **THEN** the live-scan coverage SHALL state the full check set and access to any credential-reachable repository
