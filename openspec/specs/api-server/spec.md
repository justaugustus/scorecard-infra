# api-server Specification

## Purpose
Defines the server's HTTP surface: the Scorecard-webapp-compatible
`GET /projects/{host}/{org}/{repo}` and `/badge` contract serving canonical
JSON2, the `/capabilities` endpoint that advertises this server's provenance and
caveats, `/health` and `/readyz` probes, consistent error handling, and
responsible framing on every response.
## Requirements
### Requirement: Serve results over the Scorecard-webapp GET contract

The server SHALL expose `GET /projects/{host}/{org}/{repo}` and SHALL return the
repository's Scorecard result as canonical JSON2, sourced through the result cache.

#### Scenario: Latest result requested

- **WHEN** a client requests `/projects/{host}/{org}/{repo}` without a commit
- **THEN** the server SHALL return the latest available result as JSON2

#### Scenario: Commit-pinned result requested

- **WHEN** a client requests `/projects/{host}/{org}/{repo}?commit={sha}`
- **THEN** the server SHALL return the result for that commit as JSON2

#### Scenario: Compatible with the scorecard-mcp client

- **WHEN** `scorecard-mcp` is configured with this server as its `--base-url`
- **THEN** its existing repository-score and check tools SHALL succeed unmodified

### Requirement: Badge endpoint

The server SHALL expose `GET /projects/{host}/{org}/{repo}/badge` returning an SVG
badge reflecting the repository's aggregate score.

#### Scenario: Badge requested

- **WHEN** a client requests the badge path for a repository
- **THEN** the server SHALL return an SVG badge

### Requirement: Advertise server capabilities and caveats

The server SHALL expose `GET /capabilities` describing its source/mode, the set of
checks it runs, whether coverage requires opt-in, its freshness policy, and its
caveats, so clients report provenance accurately instead of assuming public-cache
behavior.

#### Scenario: Capabilities requested

- **WHEN** a client requests `/capabilities`
- **THEN** the server SHALL return its mode (cached and live), check coverage,
  opt-in requirement, freshness policy, and caveats

### Requirement: Health and readiness endpoints

The server SHALL expose `GET /health` and `GET /readyz` for liveness and readiness
checks.

#### Scenario: Health requested

- **WHEN** a client requests `/health`
- **THEN** the server SHALL return a success status

#### Scenario: Readiness requested before dependencies are available

- **WHEN** the store or scan dependencies are not yet usable
- **THEN** `/readyz` SHALL return a not-ready status

### Requirement: Consistent error handling

The server SHALL map failure conditions to clear HTTP status codes with actionable
messages, distinguishing an unknown/never-produced result from a scan failure and a
malformed request.

#### Scenario: Malformed repository reference

- **WHEN** the request path is not a valid `{host}/{org}/{repo}` reference
- **THEN** the server SHALL return a client-error status explaining the expected form

#### Scenario: Scan failure on a cache miss

- **WHEN** a result is absent and the on-demand scan fails
- **THEN** the server SHALL return an error status without serving a partial result

### Requirement: Responsible framing on every response

Every result the server returns SHALL declare its source and completeness, and the
server SHALL NOT assert that a repository is secure or insecure.

#### Scenario: Source and completeness declared

- **WHEN** any result is returned
- **THEN** it SHALL declare whether it was cached or freshly scanned and its completeness

