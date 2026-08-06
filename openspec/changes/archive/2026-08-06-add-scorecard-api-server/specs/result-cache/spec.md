# result-cache: Added Requirements

## ADDED Requirements

### Requirement: Read-through cache over the scan engine

The orchestrator SHALL serve a stored result when one exists and is fresh, and SHALL
otherwise trigger an on-demand scan, persist the result, and return it.

#### Scenario: Cache hit, fresh

- **WHEN** a fresh result exists for the requested repository and commit
- **THEN** the orchestrator SHALL return the stored result and declare its source as cached

#### Scenario: Cache miss

- **WHEN** no result exists for the requested repository and commit
- **THEN** the orchestrator SHALL trigger a scan, persist the result, and return it declaring its source as live

### Requirement: Freshness policy

The orchestrator SHALL treat commit-pinned results as immutable and SHALL apply a
time-to-live to latest results, refreshing a latest result that has exceeded its TTL.

#### Scenario: Commit-pinned result is always fresh

- **WHEN** a commit-pinned result exists in the store
- **THEN** the orchestrator SHALL serve it without rescanning

#### Scenario: Stale latest result

- **WHEN** a latest result exists but has exceeded its TTL
- **THEN** the orchestrator SHALL trigger a refresh scan and serve the new result

### Requirement: Single-flight de-duplication

The orchestrator SHALL coalesce concurrent requests for the same repository and
commit so that at most one scan runs for a given key at a time.

#### Scenario: Concurrent identical requests

- **WHEN** multiple requests for the same repository and commit arrive while a scan is in progress
- **THEN** the orchestrator SHALL run a single scan and return its result to all waiting requests

### Requirement: Synchronous or asynchronous response

The orchestrator SHALL attempt to complete a scan synchronously within a configured
timeout and, when it cannot, SHALL indicate that the result is not yet ready so the
client can retry.

#### Scenario: Scan completes within the timeout

- **WHEN** an on-demand scan completes before the request timeout
- **THEN** the server SHALL return the result synchronously

#### Scenario: Scan exceeds the timeout

- **WHEN** an on-demand scan would exceed the request timeout
- **THEN** the server SHALL return a not-yet-ready response indicating the client should retry

### Requirement: Provenance and determinism

Every result returned SHALL include its source, the resolved commit SHA, the
generation date, and the Scorecard version, so results are reproducible and
cache-keyable per repository and commit.

#### Scenario: Provenance present

- **WHEN** any result is returned
- **THEN** it SHALL include its source, resolved commit SHA, generation date, and Scorecard version

### Requirement: Score semantics preserved

The orchestrator SHALL surface Scorecard scores unchanged and SHALL NOT recompute the
aggregate score.

#### Scenario: Inconclusive score

- **WHEN** a check or the aggregate has a score of -1
- **THEN** the result SHALL preserve -1 as inconclusive rather than converting it to a failing score
