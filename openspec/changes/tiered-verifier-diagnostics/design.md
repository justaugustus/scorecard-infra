# Design: Tier the workflow verifier's parser diagnostics

Decision IDs are **T\*** here, distinct from the other changes' **A\***,
**B\***, **C\***, **D\***, **E\***, **F\***, **I\***, **L\***, **V\***, and
**W\***.

## Context

actionlint's parser tolerates errors. Given a workflow with a key it doesn't
know, it still returns the parsed workflow, plus a `syntax-check` diagnostic
such as `unexpected key "cache-mode" for "workflow" section`, with the key's
line and column. The verifier rejected on any diagnostic, so the tolerance was
never used.

## Decisions

### T1 — Three tiers, with an allowlist rather than a denylist

1. **Reject malformed YAML**, and anything that stops the verifier's
   invariants from being checked. These diagnostics don't match the list.
2. **Reject unknown constructs**, including any new key in the Scorecard job
   that could run code or grant access. These don't match the list either.
3. **Let through reviewed keys that only remove capability**, for the
   reviewed value. `cache-mode: none` disables cache access. `read`, `write`
   and `write-only` keep or grant it, so they stay off the list.

The list lives in code (`reviewedKeys`). Each entry is a security decision,
so it gets code review, not a configuration toggle.

`cache-mode` is seeded although the deployed parser already accepts it. That
way the mechanism is exercised against the case that motivated it, not a
hypothetical one, and its tests carry #5322's verbatim diagnostic.

### T2 — Match the diagnostic to the YAML, not just the message

A message match alone would let `cache-mode: read` through, because the
diagnostic names only the key. So the workflow is parsed with
`go.yaml.in/yaml/v4`, the same YAML library actionlint uses, which keeps line
and column numbers aligned. The diagnostic passes only if the mapping key at
its exact position is the reviewed key and its scalar value is the reviewed
value. The section comes from the diagnostic, so a reviewed key in an
unreviewed place, such as a step, still rejects. If the YAML doesn't parse,
every diagnostic stands.
