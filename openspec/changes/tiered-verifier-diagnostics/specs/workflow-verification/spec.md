# workflow-verification: Tiered parser diagnostics

## ADDED Requirements

### Requirement: Only reviewed parser diagnostics are let through

The workflow verifier SHALL reject an upload on any parser diagnostic but one
kind: an unexpected-key diagnostic for a reviewed key in its reviewed section.
That exception SHALL apply only where the workflow sets the key to its reviewed
value. Reviewed keys SHALL be ones whose reviewed value only removes
capability, and SHALL be added through code review.

#### Scenario: A reviewed key with its reviewed value

- **WHEN** the parser reports `cache-mode` as an unexpected key at workflow or
  job scope, and the workflow sets it to `none` there
- **THEN** that diagnostic SHALL NOT reject the upload, and the verifier's
  other checks SHALL still apply

#### Scenario: A reviewed key with another value

- **WHEN** the parser reports `cache-mode` as unexpected and the workflow sets
  it to a value that keeps or grants access
- **THEN** the upload SHALL be rejected

#### Scenario: Any other diagnostic

- **WHEN** the parser reports malformed YAML, an unreviewed key, or a reviewed
  key outside its reviewed section
- **THEN** the upload SHALL be rejected
