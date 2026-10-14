# Tasks: Tier the workflow verifier's parser diagnostics

## 1. Implementation

- [x] 1.1 `reviewedKeys`, seeded with `cache-mode: none` at workflow and job
      scope (**T1**).
- [x] 1.2 `unreviewedDiagnostics`: keep every diagnostic unless it is a
      reviewed unexpected-key diagnostic whose YAML position holds the
      reviewed value (**T2**).
- [x] 1.3 Call it from `verifyScorecardWorkflow` before rejecting.

## 2. Verification

- [x] 2.1 Unit tests with #5322's verbatim diagnostic. Let through: workflow
      and job scope, and a quoted value. Rejected: another value, an
      unreviewed key, an unreviewed section, a mismatched position, a
      non-syntax diagnostic, and malformed YAML. Mixed lists keep the rest.
- [x] 2.2 Existing verifier fixtures still pass and fail as before,
      including the unknown-key control.
- [x] 2.3 A mutation that lets every diagnostic through fails the rejection
      cases and the invalid-workflow fixtures.
- [x] 2.4 Tests pass under Go 1.26.5 (CI) and 1.27.1.
