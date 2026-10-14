# Design: Lift the migration quarantine

Decision IDs are **L\*** here, distinct from `migrate-batch-pipeline`'s **C\***,
`migrate-api`'s **W\***, `configure-result-buckets`' **B\***,
`unify-release-versioning`'s **V\***, and the cloud-agnostic server's
**D\***/**F\***.

## Decisions

### L1 — The quarantine ended with the cutovers

The freeze and the quarantine were the means to a single acceptance test:
behavior didn't change across each cutover. Both cutovers are complete and
GCP is gone, so the test has nothing left to compare against. The guidance
saying otherwise now does harm: it steers fixes away from the code that
actually ships. `configure-result-buckets` already lifted one quarantined item
early; this lifts the rest.

### L2 — The import-edge ban goes too

**W10** gave the reason for the ban in so many words: "it is what keeps the
imported tree byte-comparable to what production runs." With no comparison
left to protect, an enforced boundary between packages in one Go module is
just friction. Removing the CI job doesn't make convergence a drive-by.
Converging the two serving-tier implementations still deserves its own change.
It just isn't blocked mechanically any more.

The isolation requirements in `migrate-api`'s and `migrate-batch-pipeline`'s
spec deltas are removed rather than reworded. Both changes are still active,
and archiving them would otherwise make the requirements canonical.

### L3 — Historical records stay as written

The two `initial-graft.md` files, completed task notes, and archived changes
describe what was true when they were written. Rewriting them would erase the
reasoning they record. Only text that reads as current guidance changes.

### L4 — `x-google-backend` is removed

The block was the Cloud Endpoints gateway's routing configuration, embedded in
the published contract. The gateway went with the GCP project.
`api.scorecard.dev` is served by Fastly in front of AWS, and nothing reads the
block. It is removed in its own commit because it touches the contract file
and the generated `embedded_spec.go`, which is regenerated with the pinned
go-swagger and checked by `swagger-verify`.

### L5 — The `api/` lint block's condition is met, but not acted on here

`.golangci.yml` scopes a narrower linter set to `api/` "until the freeze
lifts". Removing it surfaces findings that need code changes, so this change
only updates the comment and points at `migrate-api` task 5.7.
