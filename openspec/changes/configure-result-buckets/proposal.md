# Proposal: Configure the results API's buckets

## Why

The imported results API addresses its object store with three compile-time
constants: `gs://ossf-scorecard-results` twice (once on the read path, once on
the publish path) and `gs://ossf-scorecard-cron-results` on the fallback read.
Only the GCS driver is linked into the binary, so the constants are not merely
defaults — they are the only locations the binary can reach at all. A bucket URL
with any other scheme fails inside `blob.OpenBucket`, at the first request, in
production.

That was correct to leave alone during the import: `migrate-api` froze the
imported tree so that "the deployed behavior did not change" stayed a checkable
claim, and it named this remediation as a separate, already-planned change. This
is that change.

It is the last code-level dependency between the results API and one specific
object store. Everything else in the serving tier is already provider-neutral —
reads and writes go through `gocloud.dev/blob`, the CDN is Fastly and sits
outside any cloud, and the Sigstore publish path depends on Fulcio and Rekor
rather than on a hosting provider. With this change, moving the service to AWS
is a deployment-configuration exercise rather than a code change, which is what
makes the move reviewable as an operational step instead of a rewrite.

## What Changes

- Replace the three constants with two environment variables:
  **`SCORECARD_RESULTS_BUCKET_URL`** (the bucket the publish path writes and the
  read path checks first) and **`SCORECARD_CRON_RESULTS_BUCKET_URL`** (the
  weekly-scan bucket the read path falls back to).
- Default both to the URLs compiled in today, so a deployment that sets neither
  behaves exactly as it does now. The change is inert until something is
  deployed elsewhere.
- Collapse the two identical constants into **one** setting. Their equality was
  load-bearing — a POST writes the object a subsequent GET returns — and two
  variables would let an operator split them and produce a service that accepts
  uploads and then reports them missing.
- Blank-import the **S3 driver** alongside the GCS one, so an `s3://` bucket URL
  works without a further code change.
- Consolidate the driver imports and the bucket configuration into one file,
  `api/app/server/config.go`, rather than leaving them scattered across the
  three handlers. `badge.go`'s driver import was already vestigial — it performs
  a redirect and opens no bucket.
- Lift the corresponding quarantine note from `AGENTS.md` and
  `openspec/config.yaml`. The other quarantined items are untouched.

## Capabilities

- **hosted-api** — the imported API's storage locations become deployment
  configuration. The quarantine requirement that froze them is narrowed to the
  items still frozen.

## Impact

- `api/app/server/config.go` (new), `config_test.go` (new)
- `api/app/server/get_results.go`, `post_results.go`, `badge.go`
- `AGENTS.md`, `openspec/config.yaml`

No change to the OpenAPI contract, the request/response surface, the cache
directives, or the Sigstore verification rules. No new dependency: the S3 driver
is already in the module graph, used by `internal/store`.

## Non-goals

- **Choosing or provisioning the destination buckets.** This makes the location
  configurable; it does not decide what to configure it to.
- **Migrating data.** Corpus replication is tracked outside this repository.
- **Touching `cron/`.** Its own GCS write path stays quarantined under
  `migrate-batch-pipeline`'s freeze (**C11**).
- **Removing the `x-google-backend` block from `api/openapi.yaml`.** It is
  simultaneously the published contract and a deployed gateway's configuration;
  it stays quarantined.
- **Converging the two serving-tier implementations.** Still deferred.
