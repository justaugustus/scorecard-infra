# Proposal: Tier the workflow verifier's parser diagnostics

## Why

The results API verifies each uploaded workflow with actionlint, and rejects
the upload if the parser reports anything at all. When GitHub added
`cache-mode`, the deployed parser didn't know the key, and every workflow that
set `cache-mode: none` was rejected with HTTP 400
([ossf/scorecard#5322](https://github.com/ossf/scorecard/issues/5322)). The
key only removes capability, so the rejection protected nothing, and the fix
had to wait for a parser update and an API release.

The next new key will do the same. Ignoring parser errors wholesale would be
worse: a new key could expand what the job can do.

## What Changes

- **A reviewed list of capability-removing keys** whose "unexpected key"
  diagnostic is let through, for one exact value and in one named section
  (**T1**). It is seeded with `cache-mode: none` at workflow and job scope
  only, so the mechanism ships with a real case. Additions go through normal
  code review.
- **The diagnostic must match the YAML** (**T2**). A diagnostic is let
  through only if the YAML at its line and column sets that key to the
  reviewed value. Every other diagnostic still rejects the upload, as do
  malformed YAML and any other value.

## Capabilities

- **workflow-verification** (new) — which parser diagnostics reject an
  upload.

## Impact

- `api/app/server/verify_diagnostics.go`, `verify_diagnostics_test.go` (new)
- `api/app/server/verify_workflow.go` (one call)
- `go.mod`: `go.yaml.in/yaml/v4`, already in the module graph, becomes a
  direct dependency

No change to which workflows the deployed parser accepts today: it already
parses `cache-mode`.

## Non-goals

- **Relaxing any of the verifier's own checks** (permissions, runners,
  steps, containers). They run on the parsed workflow as before.
- **A configuration file or flag for the list.** Additions are security
  decisions and belong in code review.
