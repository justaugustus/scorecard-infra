# result-cache: Modified Requirements

## MODIFIED Requirements

### Requirement: Read-through cache over the scan engine

The orchestrator SHALL serve a stored result when one exists and is fresh. On a
miss or stale result, it SHALL produce a result through the configured produce
path: when the upstream fallback is active, the upstream is consulted as a tier
positioned by the fallback mode (before the scan in fetch-first; after a failed
scan in safety-net); otherwise, and whenever the fallback yields no usable
result, the orchestrator SHALL trigger an on-demand scan, persist it, and return
it. A used upstream result within the maximum age SHALL be persisted (backfilled)
tagged as upstream; a commit-pinned request SHALL NOT consult the upstream.

#### Scenario: Cache hit, fresh

- **WHEN** a fresh result exists for the requested repository and commit
- **THEN** the orchestrator SHALL return the stored result and declare its true
  source (cached for a locally-scanned entry, upstream for an upstream entry)

#### Scenario: Cache miss, fetch-first serves upstream

- **WHEN** no fresh stored result exists, the mode is fetch-first, and the
  upstream returns a usable result
- **THEN** the orchestrator SHALL return it declaring its source as upstream and
  SHALL backfill it to the store tagged as upstream

#### Scenario: Cache miss, upstream yields nothing

- **WHEN** no fresh stored result exists and the fallback yields no usable result
- **THEN** the orchestrator SHALL fall through to a live scan, persist it, and
  return it declaring its source as live

### Requirement: Freshness policy

The orchestrator SHALL treat commit-pinned results as immutable and SHALL apply a
time-to-live to latest results. Freshness of a latest result SHALL be
source-aware: a locally-scanned result ages by the latest TTL, and an
upstream-sourced result ages by the upstream maximum age, in both cases measured
against the result's generation date.

#### Scenario: Commit-pinned result is always fresh

- **WHEN** a commit-pinned result exists in the store
- **THEN** the orchestrator SHALL serve it without rescanning

#### Scenario: Stale locally-scanned latest result

- **WHEN** a locally-scanned latest result has exceeded the latest TTL
- **THEN** the orchestrator SHALL refresh it through the produce path

#### Scenario: Stale upstream latest result

- **WHEN** an upstream-sourced latest result has exceeded the maximum age
- **THEN** the orchestrator SHALL refresh it through the produce path rather than
  serve it stale
