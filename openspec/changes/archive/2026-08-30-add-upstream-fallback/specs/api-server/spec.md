# api-server: Modified Requirements

## MODIFIED Requirements

### Requirement: Advertise server capabilities and caveats

The server SHALL expose `GET /capabilities` describing its source/mode, the set of
checks it runs, whether coverage requires opt-in, its freshness policy, and its
caveats, so clients report provenance accurately instead of assuming public-cache
behavior. When an upstream fallback is enabled, `/capabilities` SHALL reflect the
fallback in its mode and SHALL include caveats describing the upstream's
limitations (opt-in-only coverage, staleness, and omitted checks).

#### Scenario: Capabilities requested, fallback disabled

- **WHEN** a client requests `/capabilities` and no upstream fallback is enabled
- **THEN** the server SHALL return its mode (cached and live), check coverage,
  opt-in requirement, freshness policy, and caveats

#### Scenario: Capabilities requested, fallback enabled

- **WHEN** a client requests `/capabilities` and an upstream fallback is enabled
- **THEN** the server SHALL indicate the fallback in its mode and SHALL include
  caveats stating that upstream results may be stale, may omit some checks, and
  cover only repositories that opted in upstream

#### Scenario: Upstream source declared on the response

- **WHEN** the server serves a result obtained from the upstream fallback
- **THEN** the response SHALL declare its source as upstream so the client does
  not report it as a local cached or live result
