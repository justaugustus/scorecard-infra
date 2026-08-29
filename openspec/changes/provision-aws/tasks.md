# Tasks: Provision the AWS serving environment in OpenTofu

Decision tags **A1**–**A16** are defined in `design.md`.

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
      * **No application secret exists.** The serving plane needs exactly one
        (**A11**), which is narrower than the GKE inventory implied.
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

- [x] 2.1 Install OpenTofu >= 1.10 (**A3**). **Upgraded 1.9.0 -> 1.12.6.**
      `use_lockfile` does not exist before 1.10; v1.9 documents only
      `dynamodb_table` and disables locking silently without it.
- [x] 2.2 Add `deploy/api/` per the layout in **A2**. `bootstrap/`,
      `modules/state-backend/`, and `modules/network/` exist; `secrets/`,
      `service/`, `edge/`, `ci-oidc/`, and the two environment roots do not yet.
      Corrected while building: **there is no tree-wide `versions.tf`.** Root
      modules do not inherit a `terraform` block from a parent directory, so
      each root carries its own.
      The version pin sits at `deploy/.opentofu-version` rather than under
      `api/`, so both planes share one number instead of two files drifting
      apart. Safe because `tenv`/`tofuenv` resolve it by searching the working
      directory, then parents, then home — verified against tenv's documented
      precedence order, not assumed.
      Verified `required_version = ">= 1.10"` actually fires — OpenTofu 1.9.0
      refuses with `Unsupported OpenTofu Core version` naming the line, rather
      than failing later and further from the cause.
- [x] 2.3 Add `deploy/api/README.md`: how to run, the one-time state bootstrap
      and why a local `terraform.tfstate` is a mistake after it, and the manual
      Netlify DNS steps (**A10**).
- [x] 2.4 Gitignore OpenTofu working state: `.terraform/`, `*.tfstate*`,
      `*.tfvars` with a `!*.tfvars.example` negation, and crash logs.
      `.terraform.lock.hcl` deliberately stays tracked; it is the
      provider-version lock and pinning it is the point.
      CI added as `.github/workflows/tofu.yml`: `tofu fmt -check -recursive` and
      `tofu validate` over **every** directory containing `.tf`, roots and
      modules alike — a module only ever checked through its caller can carry a
      schema error no caller happens to exercise.
      The workflow holds **no AWS credentials** and never plans or applies. A
      pull-request check that can reach the account is one that anyone who can
      open a pull request can make reach it. Clean under `actionlint` and
      `zizmor`.
- [x] 2.5 Common tag set via the provider's `default_tags`, carrying Project,
      Component, Environment, ManagedBy, and Source. Applied at the provider
      rather than per-resource so a new resource cannot be added untagged.

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
- [x] 4.3 **Adopt the corpus buckets as `data` sources; never as resources**
      (**A13**). Done in both environment roots. The data sources earn more than
      documentation: they fail the **plan** if a bucket name is wrong or
      unreachable, instead of letting the service deploy and 404 every request.
- [x] 4.4 The API reaches exactly two: `ossf-scorecard-results` (primary) and
      `ossf-scorecard-cron-results` (fallback). The other four belong to the
      batch plane.
      **New hazard found while writing the two roots (A16):** staging must read
      the real corpus for conformance to mean anything, but the publish path
      writes to whichever bucket it reads as primary — so a staging task with
      `PutObject` could overwrite production results with an unproven build's
      output, on buckets that have no versioning to restore from. Writes are
      gated behind `enable_publish_writes`, default false, production only.
      Staging fails the POST path by design.
- [x] 4.5 Do **not** create ECR (superseded by `ghcr.io`, #63), do **not** create
      any BigQuery equivalent, and do **not** change bucket versioning or
      lifecycle — that is live storage holding the corpus and wants its own
      decision.

## 5. Secrets (**A11**)

- [x] 5.1 `modules/secrets`: **written and validated, not applied.** Creates
      the `fastly` entry and a read policy scoped to exactly that ARN.
      **Corrected while building: the serving plane needs one secret, not
      three.** `api/` reads a single credential, `FASTLY_PURGE_TOKEN`
      (`api/app/server/post_results.go:639`); there is no GitHub App credential
      and no GitLab token anywhere in the tree. Its other three environment
      variables are configuration, not secrets.
- [x] 5.2 **Values are not carried in OpenTofu** — a value in a variable is a
      value in state. **Corrected: no `ignore_changes` is involved.** The module
      creates only `aws_secretsmanager_secret`, never
      `aws_secretsmanager_secret_version`, which is the resource that would hold
      the value. Nothing to ignore because nothing is managed. Loaded via
      `aws secretsmanager put-secret-value`.
- [ ] 5.3 Collapse the duplicated Fastly purge token to one secret. It exists in
      both Secret Manager and the GKE cluster today, and a rotation missing one
      copy leaves half the system purging with a dead token — silently, because a
      failed purge is indistinguishable from an unexpired cache.
- [x] 5.4 **Resolved: `gitlab/auth_token` is the batch plane's**, because the
      batch plane is what reads it — `api/` never does. Not created here.

## 6. Service (**A7**, **A8**)

- [x] 6.1 `modules/service`: ECS cluster, task definition, and Fargate service.
      Tasks in private subnets, **no public IP**, ALB as the only public entry.
      **Written and validated, not applied.** Deployment circuit breaker with
      rollback, so a deployment that never goes healthy reverts itself rather
      than sitting half-replaced.
- [x] 6.2 Two roles, deliberately distinct: the **execution** role belongs to
      the ECS agent (pull, logs, resolve secrets) and the **task** role is what
      the application's own S3 calls authenticate as. Conflating them would hand
      the application every secret the agent can resolve.
      **Corrected while building: the S3 grant is asymmetric** (**A14**). The
      publish path writes to the primary bucket only
      (`post_results.go:167` -> `:279`); the read path falls back to the cron
      bucket (`get_results.go:82`, `:94`). Primary gets `GetObject` +
      `PutObject`, fallback gets `GetObject`. A uniform read-only grant — which
      is what "read on the two buckets" would have produced — breaks publishing.
      `s3:ListBucket` on both is required for correctness, not tidiness: without
      it S3 answers a missing key with **403 instead of 404**, so an unscanned
      repository becomes an error rather than a not-found, and conformance
      checks that case.
- [x] 6.3 Two tasks across two AZs as an availability floor (**A8**), recorded
      as provisional in the variable's own documentation. Revisit against origin
      request rate after conformance.
- [ ] 6.4 Deploy `api/` — **not `cmd/scorecard-api`** (**A1**). Task definition
      written with the correct contract; not yet applied. Environment:
      `SCORECARD_RESULTS_BUCKET_URL`, `SCORECARD_CRON_RESULTS_BUCKET_URL`,
      `API_BASE_URL`, `FASTLY_PURGE_TOKEN`.
- [x] 6.5 Images pinned by **digest**, never tag. Enforced by a variable
      validation rejecting anything without `@sha256:<64 hex>`, so a tag is a
      plan-time error rather than a rollback that silently lands on whatever the
      tag means that day.
- [x] 6.6 CloudWatch log group with 30-day retention. The default is "never
      expire", which is a slow cost leak rather than a durability feature.

## 7. Edge (**A9**, **A10**)

- [x] 7.1 `modules/edge`: ACM certificate, ALB, target group, HTTPS listener,
      HTTP->HTTPS redirect, and both security groups. **Written and validated,
      not applied.** OpenTofu owns all of it, so the origin's lifecycle is
      independent of any workload object; deletion protection is on.
      **New finding (A15): the shipping API has no health endpoint.** Two routes
      only, no `/health`, no `/readyz`, no Docker `HEALTHCHECK` — the
      health-endpoint capability belongs to `internal/`, which is not what
      deploys. The target group therefore probes `/` and accepts any non-5xx,
      checking liveness rather than correctness. Probing a real `/projects` path
      would fold S3 into target health and let one transient S3 fault drain the
      entire pool at once.
- [ ] 7.2 **Outputs emitted** (`certificate_validation_records`, `alb_dns_name`)
      in both environment roots; creating the records in **Netlify DNS** remains
      an operational step. `aws_acm_certificate_validation` blocks
      until the validation records exist, which makes the manual step a visible
      gate.
- [ ] 7.3 Verify the origin the way Fastly will: TLS handshake against the
      hostname, valid chain, no SNI override and no disabled verification
      (**A10**).

## 8. CI (**A9**)

- [x] 8.1 `modules/ci-oidc`: **written and validated, not applied.** Trust is
      `StringEquals` on `repo:<owner>/<name>:environment:<env>` — no wildcard,
      scoped to one repository and one environment rather than the org or a
      branch, since a branch trust extends to anyone who can push one. `aud` is
      pinned so a token minted for another audience cannot be replayed.
      `iam:PassRole` is scoped to the two task roles **and** conditioned on
      `iam:PassedToService = ecs-tasks.amazonaws.com`; unscoped, it would let CI
      run a task as any role in the account.
      Deliberately application-deploy only. `tofu apply` stays a human action,
      because a role that can apply arbitrary OpenTofu can rewrite its own trust
      policy. Staging creates the provider and production consumes it — AWS
      permits one per issuer per account, and the account had none.

## 9. Verification

- [x] 9.1 **First run against real S3 is a test, not a formality.** The `s3blob`
      driver is linked and the bucket URL is configuration, but this combination
      has never been executed by anyone. Treat a failure here as expected
      information.
      **Passed 2026-08-29**, evidenced by 9.4: `results-known-good`,
      `results-high-traffic`, `results-cron-only`, and `results-gitlab` all
      returned real S3-backed 200s from the staging origin, byte-identical to
      production. The driver worked on the first real attempt.
- [ ] 9.2 Measure the ESPv2 contribution (**A12**). **Runnable today — needs no
      AWS resources**, so it does not have to wait behind the apply:
      `conformance.sh compare <scorecard-endpoints-prod> <scorecard-api-prod>`.
      **Corrected: compare the gateway against the application behind it, not
      production against staging.** An earlier version of this task said
      staging, which is confounded — finding 6.3b established production runs a
      six-month-old image, so that diff mixes the gateway's contribution with a
      deliberate code change and can attribute neither. Gateway-vs-app holds
      application, data, and version constant. `scorecard-api-prod` has ingress
      `all`, so it is directly addressable; both URLs are in the capture.
      **Origin-to-origin, never hostname-to-hostname** — finding 6.3a
      established the two public hostnames cache separately under a year-long
      `Surrogate-Control` with only one purged, so a CDN comparison measures
      cache vintage, not behavior.

      **Blocked, and treated as superseded by 9.4 rather than pursued
      further.** The one run attempted (2026-08-29) showed 20 of 20 requests
      differing, with `scorecard-api-prod` returning a uniform 403 on every
      path including `results-known-good` — not the selective
      undocumented-path rejection this task is meant to isolate.
      `get-iam-policy` on the service confirmed why: its only
      `roles/run.invoker` binding is
      `367732848534-compute@developer.gserviceaccount.com` (the project's
      default Compute SA, not a purpose-built identity), no `allUsers` — so
      the run measured Cloud Run's auth gate, not ESPv2. Minting a token for
      that service account requires impersonating it, which requires
      `roles/iam.serviceAccountTokenCreator` on it; the account available in
      this session does not have that binding, and getting it requires
      another admin's action.
      Not pursued further because 9.4 already produced a more direct answer:
      its `cors-preflight` finding shows the gateway rejecting a valid OPTIONS
      request with its own stale-contract error, while the identical
      application code (minus the gateway) answers it correctly. That is
      concrete, attributed evidence of ESPv2 breaking a real request, obtained
      without impersonation — sufficient for the **A12** decision on its own.
- [x] 9.3 Record the diff and decide per difference: fold into the application,
      or accept deliberately. Do not let the cutover be where this is found.
      **Both differences from 9.4 accepted deliberately, not folded in** —
      production is due to redeploy on current `main` regardless of cloud, so
      matching today's stale gateway/app behavior on the way out is not the
      target.
- [x] 9.4 Run conformance against the AWS origin. Cover the two-bucket read
      fallback, absent and malformed input, path traversal, every badge style,
      and the CORS headers the website depends on.
      **Run 2026-08-29** against the real staging origin (not the CDN
      hostname): `conformance.sh compare
      https://scorecard-endpoints-prod-...run.app
      https://origin-staging.scorecard.dev`. Base URLs must carry an explicit
      scheme — see the `require_scheme` fix below; a bare hostname silently
      compared two ALB HTTP-redirect pages instead of the application on the
      first two attempts.
      16 of 20 identical. Two categories of difference, neither an AWS-side
      defect:
      * **`cors-preflight`: 405 vs 204.** Production's 405 body reads "the
        current request is matched to the defined url template ... but its
        http method is not allowed" — ESPv2's own error format, not
        go-swagger's, and consistent with the gateway's OpenAPI contract being
        stale since 2023-08-30 (design.md). The application itself handles
        CORS preflight correctly: `configure_scorecard.go:106` wraps every
        route in `cors.Default().Handler(...)`, which is exactly what answered
        with 204 once the gateway wasn't in front of it.
      * **Missing `content-type` on `results-absent`, `results-bad-platform`,
        `results-unknown-commit`** (present on `results-missing-repo` and
        `results-traversal`). Traced to routing, not randomness: the first
        three match `/projects/{platform}/{org}/{repo}` and reach the
        operation-specific `GetResultHandler` -> `NewGetResultNotFound()`
        (`get_results.go:44-51`), which never explicitly sets `Content-Type`;
        `results-missing-repo` (too few path segments) and `results-traversal`
        (breaks the `/projects/` prefix once cleaned) never match that route
        and hit the router's generic not-found instead. Plausibly the same
        six-month staleness already logged in finding 6.3b — not confirmed by
        diffing dependency versions.
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
