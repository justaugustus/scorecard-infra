# Design: the AWS serving environment

Decision tags **A1**–**A16**. They are referenced from `tasks.md` and should be
cited in commit bodies, following the convention the other changes use.

## What the account actually contains

Captured 2026-08-29 with `scripts/cutover/capture-aws.sh`, across two runs — the
second off the TLS-intercepting network that cost the first one a bucket.
Everything below is observed, not assumed.

**Region: `us-east-1`.** All seven buckets are there. This settles **A5** — the
region was an observation waiting to be made, not a choice.

**The corpus buckets already exist, under their GCS names:**
`ossf-scorecard-results`, `ossf-scorecard-cron-results`, `ossf-scorecard-data2`,
`ossf-scorecard-rawdata`, `ossf-scorecard-input-projects`,
`ossf-scorecard-cii-data`. One further bucket in the account is unrelated to the
corpus and out of scope here.

**The copy is running through AWS DataSync.** All 17 Secrets Manager entries are
`aws-datasync!loc-*` location secrets. Nothing else is in Secrets Manager, so
**no application secret exists yet.** The serving plane needs exactly one of
them — see **A11**, which is narrower than the GKE inventory suggested.

**There is no compute infrastructure at all.** Zero EC2 instances, zero EKS
clusters, zero load balancers, zero ACM certificates, zero NAT gateways, zero
Elastic IPs, zero VPC endpoints, zero IAM OIDC providers, zero Route 53 zones.
The only network is the **default VPC** (`172.31.0.0/16`, six subnets, one per
AZ, public by default).

Two of those deserve to be stated rather than skimmed:

- **The EC2 instance this change expected to find is not in this account.** The
  initial API test either lives elsewhere or has been torn down. Nothing here
  builds on it.
- **No Route 53 zones**, which confirms the DNS delegation to Netlify from the
  other direction rather than taking it on trust.

**No application IAM roles exist either.** All 21 roles in the account are
service-linked or DataSync-created — one `AWSDataSyncS3BucketAccess-*` per
bucket, plus the Application Migration Service set. The task role, the execution
role, and the CI deploy role are all new. The Elastic IP quota is 5, which is
the ceiling both planes share for NAT gateways.

One SQS queue exists, `openssf-scorecard`. It belongs to the batch plane, which
this change does not touch.

## Superseded assumptions

This change inherits an architecture from earlier migration planning, and much
of it stands. Six assumptions do not. Each was checked against this repository
or the capture rather than reasoned about, because every one of them, followed,
produces working infrastructure for the wrong thing:

| Assumption | Actual state, verified | Consequence |
|---|---|---|
| Deploy "the existing provider-neutral API", `cmd/scorecard-api` | AGENTS.md: "`api/` is what deploys". `api/app/server/config.go:26` links `s3blob`; lines 31-32 define both bucket URL variables | Following it builds infrastructure for the wrong binary |
| GCR maps to ECR | Images publish to `ghcr.io` (#63, merged) | No registry is created |
| DNS hosting stays put, as a separate change | Zones delegated to Netlify (#57); zero Route 53 zones in the account | Records go to Netlify, not Route 53 |
| `cron/` is already S3-capable | `grep s3blob cron/` returns nothing | The batch plane cannot be provisioned yet |
| Create new buckets named `scorecard-*-<account>-<region>` | The corpus buckets already exist under their GCS names, with DataSync writing to them | Adopt, do not create (**A13**) |
| One EKS cluster shared by the API and the batch pipeline | The two planes are being deployed separately | The API does not need Kubernetes at all (**A7**) |

**A1 — the deployed binary is `api/`.** Not `cmd/scorecard-api`. This is the
single most consequential correction: it determines the container image, the
environment contract, the health endpoints, and what conformance measures. The
provider-agnostic server in `internal/` remains built and tested and off the
deployment path, exactly as AGENTS.md describes.

## Placement and layout

**A2 — the OpenTofu lives in this repository, under `deploy/`, split by what is
deployed.**

OpenTofu's [Standard Module Structure][sms] specifies a root module at the
repository root with `modules/` and `examples/` beside it, and distinguishes
public modules from internal ones by whether they carry a `README.md`. That
guidance is written for reusable modules published to a registry. Ours are
internal to one deployment, so it applies to each module under `modules/` and
not to the repository root; nothing upstream argues for a separate repository
for a deployment root.

Keeping it in this repository means the same reviewers and the same OpenSpec
process, and `openspec/config.yaml` already describes this repository as
Scorecard's hosted infrastructure "consolidated into one repository." AGENTS.md's
rule that deployment glue lives elsewhere is the no-employer-internal-references
rule, which OpenSSF's own AWS account is not. The public-repository objection is
about topology rather than credentials: state lives in S3 and secret values in
Secrets Manager, so neither is in the tree.

`deploy/` rather than `tofu/` because the split that matters is **what is
deployed**, not which tool deploys it — the serving and batch planes are
deliberately different, and the batch plane may well use Kubernetes and
Kustomize where the serving plane uses neither. A tool-named directory would
start lying the moment that happened. It also mirrors the existing top-level
`api/` and `cron/` split one level down.

```text
deploy/
  .opentofu-version           # both planes; version managers search upward
  api/                        # the serving plane; this change
    README.md
    bootstrap/                # root module; one-time, creates the state bucket
    modules/
      state-backend/          # used only by bootstrap/; see A4
      network/                # VPC, private subnets, NAT, S3 gateway endpoint
      secrets/                # Secrets Manager entries; values out-of-band
      service/                # ECS cluster, task definition, Fargate service
      edge/                   # ACM certificate, ALB, target group, listener
      ci-oidc/                # GitHub Actions OIDC provider and deploy role
    environments/
      staging/                # root module; own state key
      production/             # root module; own state key
  cron/                       # the batch plane; NOT this change
```

**Every root module carries its own `terraform` block and provider
configuration.** There is no tree-wide `versions.tf`, because OpenTofu does not
inherit one from a parent directory — `bootstrap/`, `environments/staging/`, and
`environments/production/` are three independent roots. The duplication is
required by the tool, not an oversight.

`bootstrap/` is a root module rather than an environment because the state
bucket is shared by both environments and created once, before either exists.

There is **no `modules/storage/`**: the buckets are adopted, not created
(**A13**). There is no Kustomize overlay and no Kubernetes anywhere in the
serving plane (**A7**).

[sms]: https://opentofu.org/docs/language/modules/develop/structure/

**A3 — OpenTofu >= 1.10, pinned.** `use_lockfile` does not exist before 1.10:
the [v1.9 S3 backend docs][v19] document only `dynamodb_table` and state that
locking is disabled without it, while [v1.10][v110] adds `use_lockfile`. Latest
stable is 1.12.6, and it is what is installed. Each root module declares the
constraint in its own `terraform` block, and `deploy/.opentofu-version` pins the
exact version for both planes — one file rather than one per plane, since
`tenv` and `tofuenv` resolve it by searching upward from the working directory.

Verified rather than assumed: OpenTofu 1.9.0 against this constraint fails with
`Unsupported OpenTofu Core version`, naming the line. That is the point — the
requirement surfaces as a readable error at plan time rather than as a confusing
backend failure at `init`.

[v19]: https://opentofu.org/docs/v1.9/language/settings/backends/s3/
[v110]: https://opentofu.org/docs/v1.10/language/settings/backends/s3/

## State

**A4 — S3 bucket with `use_lockfile = true`; no DynamoDB table.** Locking is a
conditional `If-None-Match` write against an object in the same bucket, which
removes a resource to create, own, pay for, and forget to grant access to.
Versioning is enabled on the state bucket: upstream advises it explicitly, and
it is the recovery path for a corrupted or truncated state.

The bootstrap is circular — the bucket holding the state must exist before there
is a backend to record its own creation. Resolution: `modules/state-backend` is
applied once with local state, then its own state is migrated into the bucket it
just created, and the local file is deleted. This is a one-time operation,
recorded as such in `deploy/api/README.md`, because the next person to see a
`terraform.tfstate` in a working tree should know whether it is a mistake.

Separate state keys per environment, one bucket. Not workspaces: workspaces
share a backend configuration and make it possible to apply to production while
believing you are in staging, whereas separate root modules under
`environments/` make the target textual and reviewable.

## Region

**A5 — `us-east-1`, because that is where the buckets are.**

Confirmed by the capture rather than chosen. This is not tidiness: the S3
gateway endpoint is regional and free, and it is the path from the VPC to S3
that avoids NAT. A service in a different region from its buckets reaches them
over NAT instead, paying per-GB egress on every object read — on a service whose
entire workload is object reads. The account has **no VPC endpoints today**, so
this is something to build, not something to inherit.

## Discovery before provisioning

**A6 — `scripts/cutover/capture-aws.sh` runs before the first `apply`, and again
when the account is expected to have changed.**

It has now run once. It was right to insist on this: the run overturned the
assumed bucket naming, revealed DataSync as the copy mechanism, and established
that the EC2 instance this change expected to find is not in the account.

It also exposed three defects in the script itself, which is the outcome the
GCP captures trained us to expect. All three are fixed:

1. The AWS CLI emits a blank line before its error text, so `head -1` on the
   error file printed nothing — seven sections reported FAILED with no reason
   beside any of them.
2. `GetBucketLifecycleConfiguration` and `GetBucketPolicy` raise an error to mean
   "not configured". Every bucket was therefore marked FAILED for the
   unremarkable property of having no lifecycle rule, which buried the one
   failure that was real.
3. A successful call returning an empty list was reported as plain `ok`, making
   an account with no compute in it look identical to one full of it. Sections
   now carry item counts.

**Run it off any TLS-intercepting network.** A VPN that re-signs certificates
produces `SSL validation failed ... self-signed certificate in certificate
chain`, which in the first run took out every call against one bucket while its
neighbours succeeded. That shape reads as a permissions problem and is not one.
A section that fails this way has established nothing about the account.

## Compute

**A7 — the serving and batch planes are separate deployments. The API runs on
ECS Fargate behind an ALB; there is no Kubernetes in the serving plane.**

The earlier design put both planes on one EKS cluster, reasoning that the batch
pipeline is already built on CronJob, Deployment, and RBAC. The premise held;
the conclusion does not survive separating the two:

- **The API takes traffic from the internet. The batch pipeline does not.** They
  have different exposure, so they want different blast radii. A shared cluster
  couples the internet-facing component to the one with the broader cloud
  permissions and the more complicated failure modes.
- **The batch plane is the harder system** — queue semantics, visibility
  heartbeats, shard completion, fourteen workers. Giving it its own environment
  keeps its complexity out of the path of a public API.
- **A shared cluster was the whole argument for EKS.** Without sharing, EKS
  costs roughly $73/month in control-plane charges alone for one stateless
  container, plus node patching, addon lifecycle, upgrades, and RBAC — none of
  which serve a single-service deployment.

Fargate is the smallest surface that satisfies the constraints: no OS to patch,
no nodes, no control plane to upgrade, an IAM task role as the identity, tasks
in private subnets, and the ALB as the only public entry.

**On "serverless" and traffic.** Fargate is not request-metered. It bills per
vCPU-second and GB-second of running task time, so cost is flat with respect to
request volume until another task is needed. That distinction is the point:
per-request metering is the cost problem this migration exists to escape, and
Lambda or API Gateway would reintroduce it. Fargate has no per-request charge
anywhere in it. Fastly also absorbs nearly all the traffic — finding 6.3a
established `Surrogate-Control: max-age=31557600`, so origin load is cache
misses only, a small fraction of public volume.

Where this stops being right: sustained demand above roughly 10-20 tasks, where
Fargate's per-vCPU premium over on-demand EC2 exceeds the operational cost of
managing instances. Well outside the current envelope, and a revisit rather than
a rebuild.

**A8 — start at two tasks across two AZs and tune after conformance.**
Production Cloud Run runs 1 vCPU / 512Mi at concurrency 120 with `maxScale`
1000, and scales to zero. Fargate does not scale to zero, so the Cloud Run
numbers bound the per-task request and say nothing about the floor. Two tasks
is a starting point chosen for availability, not from a measurement; the
measurement that should replace it is **origin request rate — cache misses per
second, not public traffic**. Deliberately deferred: conformance has to run
before there is anything to size against.

The API runs as the project's **default compute service account** on GCP today.
That breadth is not reproduced — the task role gets the two buckets it reads and
the secrets it needs, and nothing else.

**A9 — OpenTofu owns the ALB, the target group, and the ECS service.**

With Fargate there is no Kubernetes controller in the picture, so the ALB is a
plain resource and the ECS service registers its tasks into the target group
natively. This is strictly simpler than the Kubernetes alternative it replaces,
and it preserves the property that matters: the origin is a first-class IaC
resource with a lifecycle independent of any workload object. Fastly is pinned
to that origin, the cutover is one backend field, and rollback is restoring it.

## The origin

**A10 — the origin is a hostname with an ACM certificate, never a bare IP.**

Fastly must verify the origin certificate. An IP origin requires hand-set SNI
and `override_host` plus either a certificate issued for an IP or disabled
origin verification — a security-posture change smuggled inside a migration
whose entire acceptance claim is that behavior did not change. An ALB with an
ACM certificate satisfies this directly.

**A15 — the load balancer health check probes liveness, not correctness.**

The shipping API has **no health endpoint**. Its contract is two routes —
`/projects/{platform}/{org}/{repo}` and `.../badge` — with no `/health`, no
`/readyz`, and no `HEALTHCHECK` in its Dockerfile. Earlier planning said the API
"already supports health/readiness endpoints"; that is true of the
provider-agnostic server in `internal/`, which is not what deploys (**A1**).

So the target group probes `/` and accepts any non-5xx. The go-swagger router
answers 404 there, and that 404 is the signal: the process is up, listening, and
routing.

Probing a real `/projects` path instead would be a mistake, and an attractive
one. It would fold S3 availability into target health — and because every task
runs the same probe against the same bucket, a transient S3 problem would fail
all of them at once, drain the pool, and convert a degraded service into an
unreachable one. A load balancer's question is "should this target receive
traffic," not "is the system working." Cloud Run has no application health check
today either, so this is also the closer match to the behavior being migrated.

Worth revisiting when the freeze on `api/` lifts; adding a real health endpoint
is a code change and does not belong in this one.

The account has **no ACM certificates and no load balancers today**, so both are
built here. Because the zones are on Netlify DNS, two record sets are created
outside OpenTofu and emitted as outputs: the ACM DNS validation records, and the
origin hostname pointing at the ALB. `aws_acm_certificate_validation` blocks
until the validation records exist, which makes the manual step a visible gate
rather than a silent prerequisite.

## Storage

**A13 — the corpus buckets are adopted as data sources and never managed as
resources.**

They already exist, they hold the corpus, and DataSync is actively writing to
them. Declaring them as `aws_s3_bucket` resources would put the entire result
corpus one `tofu destroy`, one refactor, or one `-target` mistake away from
deletion — and OpenTofu cannot tell the difference between "this bucket should
not exist" and "someone removed the block."

So: `data "aws_s3_bucket"` for reference, and IAM policy granting the task role
access. The buckets' own configuration — versioning, lifecycle — is out of scope
here. The capture shows versioning is **not** enabled on any of them and no
lifecycle rules exist, which is worth addressing, but it is a change to live
storage holding the corpus and deserves its own decision rather than arriving as
a side effect of provisioning a web service.

The API needs exactly two: `ossf-scorecard-results` (primary, via
`SCORECARD_RESULTS_BUCKET_URL`) and `ossf-scorecard-cron-results` (fallback, via
`SCORECARD_CRON_RESULTS_BUCKET_URL`). The other four belong to the batch plane.

All seven buckets share one shape, confirmed by the second capture: SSE-S3
(`AES256`) with bucket keys on and `SSE-C` blocked, all four public-access
blocks on, **versioning not enabled**, and no lifecycle rules. So the corpus is
encrypted and closed to the public, and it is not protected against overwrite —
which matters more here than it would elsewhere, because `latest` is an
unconditional overwrite and a delayed run can regress it. That is a known
correctness debt, it is real, and fixing it is still not this change's business.

One item remains uncaptured: the bucket policy on `ossf-scorecard-results`, lost
to a lingering TLS failure. The gap is bounded rather than open —
`BlockPublicPolicy` and `RestrictPublicBuckets` are both on, so S3 rejects a
public policy outright. Whatever that policy says, it is not granting public
access.

## Secrets

**A11 — Secrets Manager holds the values; OpenTofu creates the containers.**

**The serving plane needs exactly one secret**, and this was checked in the code
rather than inherited from the GKE inventory. `api/` reads one credential:
`FASTLY_PURGE_TOKEN`, at `api/app/server/post_results.go:639`. There is no
GitHub App credential and no GitLab token anywhere in the tree. Its other three
environment variables — `API_BASE_URL` and the two bucket URLs — are
configuration, not secrets.

That settles the open question about GitLab ownership: **`gitlab/auth_token` is
the batch plane's**, because the batch plane is what reads it. It gates GitLab
scanning and there is a `cron/internal/data/gitlab-projects-releasetest.csv`
lane, so it is required — just not here. Creating it in both places would be
worse than creating it in either.

OpenTofu creates the secret *resource* and the IAM policy granting read on
exactly that ARN. It does **not** create an `aws_secretsmanager_secret_version`,
which is the resource that would carry the value: a value passed to OpenTofu is
a value written to state, and the state bucket would then be a credential store
with weaker controls than the service built for the purpose. Because the version
resource is simply absent, there is no value in state and **nothing to
`ignore_changes` on** — an earlier draft of this design said otherwise and was
wrong about which resource holds the value.

**The duplicated Fastly purge token collapses to one secret.** It exists in both
Secret Manager and the GKE cluster today, and a rotation that misses one copy
leaves half the system purging with a dead token — silently, because a failed
purge is indistinguishable from a cache that has not expired.

**A14 — the task role's S3 grant is asymmetric, because the code is.** The
publish path writes to the primary bucket only
(`post_results.go:167` → `:279`); the read path tries the primary and falls back
to the cron bucket (`get_results.go:82`, `:94`). So the primary gets
`GetObject` + `PutObject` and the fallback gets `GetObject`. A uniform grant
would either break publishing or hand out a write nothing uses.

`s3:ListBucket` is granted on both, and it is not incidental: without it S3
answers a `GET` for a missing key with **403 rather than 404**, so a repository
that has never been scanned would surface as an error instead of "not found" —
and that is a case the conformance harness checks.

**A16 — staging and production share the corpus for reads; only production may
write to it.**

This one only became visible when the two environment roots were written
side by side, and it is the sharpest hazard in the design.

Staging has to read the *real* corpus. Conformance against an empty bucket
proves nothing: every request would 404 identically whether the service worked
or not, and the harness's whole value is comparing real responses to production.
So both environments point at `ossf-scorecard-results`.

But the API's publish path writes into whichever bucket it reads as primary. A
staging task granted `PutObject` on that bucket could therefore overwrite
production results with output from an unproven build — and because `latest` is
an unconditional overwrite on buckets that have **no versioning enabled**, there
would be nothing to restore from.

So the write is gated behind `enable_publish_writes`, defaulting to **false**,
and only the production root sets it true. Staging consequently fails the POST
path by design. That costs nothing: the conformance harness is GET requests, and
the publish path is gated separately by watching a real Action upload land
(`migrate-api` task 6.6).

The general form of the rule is worth stating, because the batch plane will meet
it too: **sharing a data store between environments is safe in one direction
only.**

## The gateway

**A12 — drop ESPv2, but measure it first.**

The evidence that it enforces nothing is good: `api/openapi.yaml` has no
`securityDefinitions`, no API key requirement, and `x-google-allow: all` (lines
29 and 32). It is a routing shim, and staging already runs without it.

The evidence is not complete, and the gap is specific: a gateway validating
requests against an OpenAPI contract rejects undocumented paths and malformed
parameters *before* they reach the application, so removing it can change status
codes on exactly the inputs nobody routes deliberately. The production gateway
also enforces a 2023-08-30 configuration, so what it validates against is not
what `openapi.yaml` currently says.

The measurement is cheap and the harness exists:

```sh
scripts/api-conformance/conformance.sh compare \
  <scorecard-endpoints-prod URL> \
  <scorecard-api-prod URL>
```

**Compare the gateway against the application behind it, not production against
staging.** An earlier draft of this design said staging, and that comparison is
confounded: finding 6.3b established that production runs a six-month-old image,
so a prod-vs-staging diff mixes the gateway's contribution with a deliberate
code change and cannot attribute either. Putting the gateway and the service it
fronts side by side holds the application, the data, and the version constant,
leaving ESPv2 as the only variable. The capture confirms `scorecard-api-prod`
runs with ingress `all`, so it is directly addressable; both URLs are in the
gitignored capture output.

Run it **origin-to-origin, never hostname-to-hostname**. Finding 6.3a
established that the two public hostnames cache separately under a year-long
`Surrogate-Control` and only one is purged, so a CDN-level comparison measures
cache vintage rather than behavior.

Whatever the diff shows is either folded into the application or accepted
deliberately. The point is that the cutover should not be the first time anyone
finds out. This needs no AWS resources and can run today.

## What this change does not decide

- **Whether production drops the gateway at the same instant as the origin.**
  A12 settles that it goes, not when. Dropping it against GCP first would leave
  the cutover changing exactly one variable, at the cost of touching production
  before the migration. That sequencing belongs to `migrate-api` group 6,
  informed by the measurement.
- **Task count and size** (A8), pending an origin-rate measurement.
- **Whether the corpus buckets get versioning and lifecycle rules** (A13). They
  have neither. That is a change to live storage and wants its own decision.
- **Anything about the batch plane.** Its queue already exists; its S3 and SQS
  drivers do not (`grep s3blob cron/` returns nothing). The SQS visibility
  heartbeat is the highest-risk item in the whole migration and it is a code
  change in `cron/`, not infrastructure. It belongs to
  `migrate-batch-pipeline`.
