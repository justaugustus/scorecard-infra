# Tasks: Configure the results API's buckets

## 1. Configuration

- [x] 1.1 Add `api/app/server/config.go` holding the two environment variable
      names, their defaults, and the accessors `resultsBucketURL()` and
      `cronResultsBucketURL()` (**B1**, **B2**).
- [x] 1.2 Treat an empty environment value as unset (**B3**).
- [x] 1.3 Blank-import `gocloud.dev/blob/s3blob` alongside `gcsblob` in
      `config.go` (**B5**).

## 2. Call sites

- [x] 2.1 `get_results.go`: drop the two constants and the driver import; call
      the accessors. Reword the log line and comment, which named GCS
      specifically.
- [x] 2.2 `post_results.go`: drop `resultsBucket`; call `resultsBucketURL()`
      (**B4**).
- [x] 2.3 `badge.go`: drop the vestigial driver import — the handler issues a
      redirect and opens no bucket (**B5**).
- [x] 2.4 Confirm no other reference to the constants survives, including in
      tests and generated code.
- [x] 2.5 **Left alone deliberately:** `errWritingBucket`'s text still says
      "error writing to GCS bucket", which is inaccurate once the target is not
      GCS. It is wrapped into the publish path's error response, so rewording it
      changes an observable string on the one path the conformance harness
      cannot exercise. Not worth spending the freeze's credibility on a noun.
      Reword it with the runbook, or when the POST path next changes for a
      reason of its own.

## 3. Verification

- [x] 3.1 `go build ./...` and `go vet ./api/...` clean.
      Note: the first attempt reported a false pass — `head` in the pipeline
      masked a nonzero exit, and the real failure was a `GOROOT` pointing at a
      different Go than the one on `PATH`, not the change.
- [x] 3.2 `go test ./api/...` — all packages pass.
- [x] 3.3 `go mod tidy` produces no diff. The S3 driver was already in the
      module graph via `internal/store`, so this adds no dependency.
- [x] 3.4 `golangci-lint run ./...` — 0 issues.
- [x] 3.5 Prove both drivers are linked into the API binary:
      `go list -deps ./api | grep -E 'blob/(gcsblob|s3blob)$'` returns both.
      This is the check that matters for **B5** — a driver that compiles but is
      not linked fails only at run time, on the first request that uses its
      scheme.
- [x] 3.6 Unit tests in `config_test.go` covering: defaults when unset (against
      literal strings, per **B2**), `s3://` and `file://` overrides, empty-means-
      unset, and the read/write bucket invariant from **B4**.
- [ ] 3.7 **Not covered here: a real read from a real S3 bucket.** Deferred to
      staging deployment, which needs a bucket, credentials, and an IAM policy
      that do not exist yet. This change is verified as *configurable*, not as
      *working on AWS* — do not report it as the latter.

## 4. Documentation

- [x] 4.1 Remove the bucket entry from the quarantine list in `AGENTS.md`,
      leaving the other three (**B7**).
- [x] 4.2 Same in `openspec/config.yaml`'s context block.
- [ ] 4.3 Document the two variables where an operator will look for them,
      alongside `API_BASE_URL` and `FASTLY_PURGE_TOKEN`. There is no deployment
      runbook in this repository yet; this lands with the one the cutover needs.

## 5. Follow-on, not in this change

- [ ] 5.1 Set the variables to `s3://…` in the staging deployment and run
      `scripts/api-conformance/conformance.sh compare` against production.
      A body difference with matching cache diagnostics means the buckets hold
      different data — a replication gap, not an API defect (harness triage
      step 2).
- [ ] 5.2 `cron/`'s own GCS write path, once `migrate-batch-pipeline`'s freeze
      lifts (**B6**, **C11**).
