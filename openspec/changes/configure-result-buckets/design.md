# Design: Configure the results API's buckets

Decision IDs are **B\*** here, distinct from `migrate-batch-pipeline`'s **C\***,
`migrate-api`'s **W\***, and the cloud-agnostic server's **D\***/**F\***.

## Context

This is the follow-on `migrate-api` anticipated. That change deliberately did not
touch the imported tree's storage coupling, because its acceptance test was that
deployed behavior did not change, and the cheapest way to keep that claim
checkable was to change nothing. The import having landed and been verified, the
freeze can be lifted for this one item without weakening that claim — provided
the lift is itself a no-op by default, which is **B2**.

## Decisions

### B1 — Environment variables, not a config file or flags

The service already reads `API_BASE_URL`, `FASTLY_PURGE_TOKEN`, and
`STORAGE_EMULATOR_HOST` from the environment, and it runs as a container on a
platform where environment is the native configuration channel. Adding a config
file or a flag parser would introduce a second mechanism for no benefit and
would need its own precedence rules against the first.

Names are `SCORECARD_RESULTS_BUCKET_URL` and `SCORECARD_CRON_RESULTS_BUCKET_URL`
— prefixed, and explicitly `_URL` because the value is a full
`gocloud.dev/blob` URL including scheme and query parameters (`s3://bucket?region=…`),
not a bare bucket name. A variable called `..._BUCKET` invites `ossf-scorecard-results`,
which fails at `blob.OpenBucket` with a message about a missing scheme.

### B2 — Defaults preserve today's behavior exactly

Both variables default to the URL that was compiled in. An existing deployment
that sets neither is byte-for-byte unaffected, so this change can land and ship
ahead of any hosting decision, and it cannot be the cause of a cutover incident.

The unit test asserts the defaults against **literal strings** rather than
against the default constants. Comparing a constant to itself passes even after
someone edits it, which is precisely the regression worth catching.

### B3 — An empty value means unset

Container platforms readily inject an empty string for a variable that was
declared but left blank — a templated manifest with an unbound substitution is
the common case. An empty bucket URL is never meaningful configuration, so
falling through to the default is strictly better than honoring it: the
alternative turns a typo in a deployment manifest into a service that opens no
bucket and reports every repository missing.

### B4 — One setting for the primary bucket, not two

`get_results.go` and `post_results.go` held separate constants with the same
value, and the sameness was not a coincidence — the publish path writes the
object the read path returns. Exposing two variables would let an operator set
them to different buckets and get a service that accepts an upload, returns 200,
purges the CDN, and then serves 404 for the result it just stored. Nothing in
the code would fail; the symptom would surface as "the Action says it published
but the badge is stale," which is an expensive thing to debug.

The cron bucket stays separate because it genuinely is separate: a different
system writes it, and the read path treats it as a fallback rather than a peer.

### B5 — Link the S3 driver in, and do it in one place

`gocloud.dev/blob` selects a driver by URL scheme at run time, from a registry
populated by package `init`. A driver that is not blank-imported is not a
missing feature — it is a run-time failure at the first request that uses its
scheme. So making the URL configurable without linking S3 in would ship a
setting that cannot be set to the thing it exists for.

The imports move to `api/app/server/config.go`, next to the configuration that
determines which scheme will actually be used. Scattering blank imports across
handlers is how `badge.go` ended up importing a storage driver it never uses:
the file performs an HTTP redirect and opens no bucket. Keeping them in one file
makes the set of reachable backends a single, reviewable list.

This matches the repository's existing rule — `internal/store` blank-imports
every backend for the same reason — so it converges the imported tree toward
house style rather than adding a second convention.

### B6 — Scope stops at the results API

`cron/` also hardcodes a GCS write path and is also a lift-and-shift candidate,
but it is still under `migrate-batch-pipeline`'s freeze and its cutover has a
different timeline. Bundling them would make this diff unreviewable as a
statement about the serving tier and would put a frozen tree in a change that
must ship this week.

### B7 — Narrow the quarantine note rather than deleting it

`AGENTS.md` and `openspec/config.yaml` list four quarantined items. Only one is
being remediated. Deleting the whole block would read as "the freeze is over"
and invite exactly the opportunistic fixes the block exists to prevent, so the
bucket entry is removed and the remaining three stay as written.

## Risks

**The two bucket variables can be pointed at buckets whose contents differ.**
B4 removes the read/write split, but not the possibility of configuring a
primary bucket that has not finished replicating. That is a data-migration
concern rather than a code one, and the conformance harness
(`scripts/api-conformance/`) is the check for it: a result body difference with
matching cache diagnostics means the two deployments are reading different data,
which is step 2 of its documented triage.

**Nothing here has been exercised against a real S3 bucket.** The driver is
proven linked, and the URL is proven to reach `blob.OpenBucket` unmodified, but
the first genuine end-to-end read from S3 will happen at staging deployment.
That is the right place for it — it needs a bucket, credentials, and an IAM
policy that do not exist yet — but it means this change is verified as
*configurable*, not as *working on AWS*.
