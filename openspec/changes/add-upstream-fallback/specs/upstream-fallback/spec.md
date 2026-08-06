# upstream-fallback: Added Requirements

## ADDED Requirements

### Requirement: Optional upstream fallback tier

The server SHALL support an optional upstream fallback that fetches an existing
`latest` result from a configured upstream Scorecard API. It SHALL be active only
when an upstream URL is configured and the `fallback.enabled` flag is on, so it
can be disabled at runtime without a redeploy.

#### Scenario: Inactive without a URL

- **WHEN** no upstream URL is configured
- **THEN** the orchestrator SHALL serve only from the local store and live scans

#### Scenario: Runtime kill-switch

- **WHEN** an upstream URL is configured but the `fallback.enabled` flag is off
- **THEN** the orchestrator SHALL NOT consult the upstream

### Requirement: Fallback ordering modes

The fallback SHALL support two orderings selected by the `fallback.mode` flag,
defaulting to fetch-first.

#### Scenario: Fetch-first mode

- **WHEN** the mode is fetch-first and a request misses the local cache
- **THEN** the orchestrator SHALL consult the upstream first and SHALL fall
  through to a live scan only when the upstream has no usable result

#### Scenario: Safety-net mode

- **WHEN** the mode is safety-net and a request misses the local cache
- **THEN** the orchestrator SHALL attempt a live scan first and SHALL consult the
  upstream only when the scan cannot produce a result

#### Scenario: Default mode

- **WHEN** the fallback is active but no mode is configured
- **THEN** the orchestrator SHALL use fetch-first

### Requirement: Upstream staleness bound

An upstream result older than the configured maximum age SHALL NOT be used; the
request SHALL fall through as though the upstream had no result.

#### Scenario: Upstream result within max-age

- **WHEN** the upstream returns a result whose generation date is within max-age
- **THEN** the orchestrator MAY serve and backfill it

#### Scenario: Upstream result older than max-age

- **WHEN** the upstream returns a result older than max-age
- **THEN** the orchestrator SHALL treat it as no usable result and fall through

### Requirement: Best-effort, non-fatal fetch

An upstream fetch SHALL be best-effort and bounded by a configured timeout: a
miss or any upstream error SHALL NOT fail the request but SHALL let the
orchestrator continue with the active mode's next step.

#### Scenario: Upstream unreachable or errors

- **WHEN** the upstream request times out or returns an error
- **THEN** the orchestrator SHALL log it non-fatally and continue as for a miss

### Requirement: Honest provenance for upstream results

A result served from the upstream SHALL declare its source as upstream and SHALL
carry the upstream result's own generation date. Completeness SHALL follow the
uniform, source-agnostic rule (present and conclusive checks), not a
source-specific penalty.

#### Scenario: Source and staleness declared

- **WHEN** a result is served from the upstream
- **THEN** it SHALL declare its source as upstream and report the upstream
  result's generation date rather than the time of the request

### Requirement: Commit-pinned requests bypass the fallback

Because the upstream answers only `latest`, a commit-pinned request SHALL NOT
consult the upstream in any mode.

#### Scenario: Commit-pinned request

- **WHEN** a request specifies a commit
- **THEN** the orchestrator SHALL bypass the upstream and produce the result by
  live scan
