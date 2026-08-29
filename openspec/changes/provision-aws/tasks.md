# Tasks: Provision the AWS serving environment in OpenTofu

Decision tags **A1**–**A13** are defined in `design.md`.

## 1. Discovery

- [x] 1.1 Write `scripts/cutover/capture-aws.sh` (**A6**), the AWS counterpart to
      `capture-config.sh` and `capture-fastly.sh`, adopting the conventions those
      two arrived at across five runs.
- [x] 1.2 Run it. **First real run 2026-08-29, 20 of 20 sections.** It corrected
      three assumptions in this change and exposed three defects in itself,
      which is the outcome the GCP captures trained us to expect.
- [x] 1.3 **Region: `us-east-1`** (**A5**), observed from where the buckets are
      rather than chosen.
- [x] 1.4 Reconcile the capture against this change. Result:
      * **The corpus buckets already exist** under their GCS names, in
        `us-east-1`, with **AWS DataSync** writing to them — all 17 Secrets
        Manager entries are `aws-datasync!loc-*`. Adopt, do not create
        (**A13**).
      * **No application secrets exist.** `github`, `gitlab`, and `fastly` are
        all new.
      * **No compute infrastructure exists at all** — zero EC2, EKS, load
        balancers, ACM certificates, NAT gateways, EIPs, VPC endpoints, and IAM
        OIDC providers. Only the default VPC.
      * **The EC2 instance this change expected is not in this account.** The
        initial API test lives elsewhere or is gone. Nothing here builds on it.
      * **Zero Route 53 zones**, confirming the Netlify delegation from the AWS
        side rather than on trust.
      * One SQS queue, `openssf-scorecard`, belonging to the batch plane.
- [x] 1.5 Fix the three defects the run exposed: the blank first line in AWS CLI
      error output that made seven FAILED sections reasonless; `NoSuch*` errors
      that mean "not configured" being reported as failures; and empty result
      sets reported as plain `ok`. Sections now carry item counts. Helpers
      verified against the captured files.
- [x] 1.6 **Re-captured 2026-08-29 off the intercepting network.** The three
      script fixes are confirmed working: sections now carry item counts, the
      `NoSuch*` "not configured" cases report as `none`, and the one genuine
      failure carries its reason. `ossf-scorecard-results` is now known:
      SSE-S3 (`AES256`, bucket keys on, `SSE-C` blocked), all four public-access
      blocks on, **versioning not enabled**, no lifecycle rules — the same shape
      as every other bucket.
      Its bucket *policy* is the one thing still uncaptured, on a lingering TLS
      failure. Bounded rather than open: `BlockPublicPolicy` and
      `RestrictPublicBuckets` are both on, so S3 would reject a public policy.
      Whatever it says, it is not granting public access.
- [x] 1.7 Inventory the IAM roles. All 21 are service-linked or DataSync-created
      — seven `AWSDataSyncS3BucketAccess-*`, one per bucket, plus the
      Application Migration Service set. **No application roles exist**, so the
      task role, the execution role, and the CI deploy role are all new.
      Elastic IP quota is 5, which bounds how many NAT gateways both planes can
      hold between them.

## 2. Toolchain and scaffolding

- [ ] 2.1 Install OpenTofu >= 1.10 (**A3**). `use_lockfile` does not exist
      before it; v1.9 documents only `dynamodb_table` and disables locking
      without it. Latest stable is 1.12.6.
- [x] 2.2 Add `deploy/api/` per the layout in **A2**. `bootstrap/`,
      `modules/state-backend/`, and `modules/network/` exist; `secrets/`,
      `service/`, `edge/`, `ci-oidc/`, and the two environment roots do not yet.
      Corrected while building: **there is no tree-wide `versions.tf`.** Root
      modules do not inherit a `terraform` block from a parent directory, so
      each root carries its own. `.opentofu-version` pins 1.12.6.
      Verified `required_version = ">= 1.10"` actually fires — OpenTofu 1.9.0
      refuses with `Unsupported OpenTofu Core version` naming the line, rather
      than failing later and further from the cause.
- [x] 2.3 Add `deploy/api/README.md`: how to run, the one-time state bootstrap
      and why a local `terraform.tfstate` is a mistake after it, and the manual
      Netlify DNS steps (**A10**).
- [ ] 2.4 Gitignore OpenTofu working state — **done**: `.terraform/`,
      `*.tfstate*`, `*.tfvars` with a `!*.tfvars.example` negation, and crash
      logs. `.terraform.lock.hcl` deliberately stays tracked; it is the
      provider-version lock and pinning it is the point.
      **Still open:** `tofu fmt -check` and `tofu validate` in CI.
- [ ] 2.5 Define a common tag set applied to every created resource, so anything
      outside this tree is identifiable as drift.

## 3. State backend (**A4**)

- [x] 3.1 `modules/state-backend`: versioned, encrypted, public-access-blocked S3
      bucket. No DynamoDB table. **Written and schema-validated, not applied.**
      Also carries `prevent_destroy`, a deny-insecure-transport bucket policy,
      and lifecycle rules expiring superseded state versions after 90 days and
      aborting incomplete uploads after 7.
- [ ] 3.2 Apply once with local state, migrate its own state into the bucket,
      delete the local file. Record it as one-time in the README.
- [ ] 3.3 `use_lockfile = true`, separate state keys per environment — **not
      workspaces**, which share a backend configuration and make applying to
      production while believing you are in staging possible.
- [ ] 3.4 Verify locking engages: run two concurrent plans and confirm the second
      is refused. A backend that silently fails to lock looks identical to one
      that works, until it does not.

## 4. Network and storage access

- [x] 4.1 `modules/network`: a **purpose-built VPC** — not the default one, whose
      six subnets are public. Private subnets across two AZs for the tasks,
      public subnets for the ALB, and an **S3 gateway endpoint** (**A5**).
      **Written and schema-validated, not applied.**
- [x] 4.2 One NAT gateway rather than one per AZ, behind `single_nat_gateway`,
      defaulting true. It is the largest fixed line item after compute (~$33/mo
      each) and the API's egress is Sigstore, GitHub, and Fastly purges rather
      than bulk transfer. The Elastic IP quota of 5 is a second reason not to
      spend them per-AZ by default.
      Private route tables are still created per-AZ so that flipping the flag
      later is a route change rather than re-associating every subnet.
- [ ] 4.3 **Adopt the corpus buckets as `data` sources; never as resources**
      (**A13**). Declaring them as `aws_s3_bucket` would put the corpus one
      `tofu destroy` or one deleted block away from deletion, and OpenTofu
      cannot distinguish "should not exist" from "someone removed the block."
- [ ] 4.4 The API needs exactly two: `ossf-scorecard-results` (primary) and
      `ossf-scorecard-cron-results` (fallback). Grant read on those two and
      nothing else. The other four belong to the batch plane.
- [ ] 4.5 Do **not** create ECR (superseded by `ghcr.io`, #63), do **not** create
      any BigQuery equivalent, and do **not** change bucket versioning or
      lifecycle — that is live storage holding the corpus and wants its own
      decision.

## 5. Secrets (**A11**)

- [ ] 5.1 `modules/secrets`: create `github`, `gitlab`, and `fastly` entries and
      the task-role read policy. All three are new; Secrets Manager currently
      holds only DataSync location entries.
- [ ] 5.2 **Values are not carried in OpenTofu** — a value in a variable is a
      value in state. Load out-of-band; `ignore_changes` on `secret_string`.
- [ ] 5.3 Collapse the duplicated Fastly purge token to one secret. It exists in
      both Secret Manager and the GKE cluster today, and a rotation missing one
      copy leaves half the system purging with a dead token — silently, because a
      failed purge is indistinguishable from an unexpired cache.
- [ ] 5.4 Decide whether the serving or batch plane owns `gitlab/auth_token`
      before creating it. It gates batch scanning, not serving; creating it in
      both places is worse than deciding once.

## 6. Service (**A7**, **A8**)

- [ ] 6.1 `modules/service`: ECS cluster, task definition, and Fargate service.
      Tasks in private subnets, **no public IP**, ALB as the only public entry.
- [ ] 6.2 One IAM task role for the API: read on the two buckets and its own
      secrets, **nothing else**. Do not reproduce the breadth of the default
      compute service account the Cloud Run service runs as today.
- [ ] 6.3 Two tasks across two AZs as an availability floor (**A8**). This is
      not a measurement — record it as provisional and revisit against origin
      request rate after conformance.
- [ ] 6.4 Deploy `api/` — **not `cmd/scorecard-api`** (**A1**). Environment:
      `SCORECARD_RESULTS_BUCKET_URL`, `SCORECARD_CRON_RESULTS_BUCKET_URL`,
      `API_BASE_URL`, `FASTLY_PURGE_TOKEN`.
- [ ] 6.5 Images pinned by **digest**, never tag (`main` for staging, `api/v*`
      for production).
- [ ] 6.6 CloudWatch log group with a retention policy. Retention defaults to
      "forever", which is a slow cost leak rather than a durability feature.

## 7. Edge (**A9**, **A10**)

- [ ] 7.1 `modules/edge`: ACM certificate, ALB, target group, HTTPS listener.
      OpenTofu owns all of it, so the origin's lifecycle is independent of any
      workload object — the cutover and its rollback both turn on that hostname.
      The account has no certificates and no load balancers today.
- [ ] 7.2 Emit the ACM validation records and the origin CNAME as outputs and
      create them in **Netlify DNS**. `aws_acm_certificate_validation` blocks
      until the validation records exist, which makes the manual step a visible
      gate.
- [ ] 7.3 Verify the origin the way Fastly will: TLS handshake against the
      hostname, valid chain, no SNI override and no disabled verification
      (**A10**).

## 8. CI (**A9**)

- [ ] 8.1 `modules/ci-oidc`: GitHub Actions OIDC provider and a deploy role whose
      trust policy constrains the `sub` claim to this repository and a protected
      environment — not to the org, and not to any branch. No provider exists in
      the account today.

## 9. Verification

- [ ] 9.1 **First run against real S3 is a test, not a formality.** The `s3blob`
      driver is linked and the bucket URL is configuration, but this combination
      has never been executed by anyone. Treat a failure here as expected
      information.
- [ ] 9.2 Measure the ESPv2 contribution (**A12**): run
      `scripts/api-conformance/conformance.sh` between the gatewayed production
      origin and the gateway-less staging origin. **Origin-to-origin, not
      hostname-to-hostname** — finding 6.3a established the two public hostnames
      cache separately under a year-long `Surrogate-Control` with only one
      purged, so a CDN comparison measures cache vintage, not behavior.
- [ ] 9.3 Record the diff and decide per difference: fold into the application,
      or accept deliberately. Do not let the cutover be where this is found.
- [ ] 9.4 Run conformance against the AWS origin. Cover the two-bucket read
      fallback, absent and malformed input, path traversal, every badge style,
      and the CORS headers the website depends on.
- [ ] 9.5 Verify the object key contract is preserved exactly:
      `{host}/{org}/{repo}/results.json` and
      `{host}/{org}/{repo}/{commit}/results.json`.
- [ ] 9.6 Compare `Content-Type` and `Cache-Control` against production before
      concluding parity. The batch helper calls `NewWriter(..., nil)` and does
      not deliberately set them, so what production serves must be observed
      rather than derived.
- [ ] 9.7 Confirm the task role **cannot** reach a bucket or secret outside its
      grant. A permissions test that only shows the allowed path working proves
      half of what is needed.
- [ ] 9.8 Record actual cost after a week against the ~$110-145/month estimate,
      and confirm no per-request charge appears anywhere in the bill.

## 10. Closeout

- [ ] 10.1 `openspec validate provision-aws --strict` passes.
- [ ] 10.2 `tofu fmt -check`, `tofu validate`, and `make lint` clean; workflows
      pass `actionlint` and `zizmor`.
- [ ] 10.3 Confirm no provider account identifier appears in any **commit
      message** — AWS account numbers, Fastly service IDs, queue URLs that embed
      an account ID. Tracked files are fine; the message cannot be revised later.
- [ ] 10.4 Confirm nothing committed names the destination operator or the reason
      for the transition. Motivation is provider-agnosticism.
- [ ] 10.5 Hand off to `migrate-api` group 6 with the staging origin proven, and
      to `migrate-batch-pipeline` with `deploy/cron/` reserved and unbuilt.
