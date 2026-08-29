# Design: the AWS serving environment

Decision tags **A1**–**A13**. They are referenced from `tasks.md` and should be
cited in commit bodies, following the convention the other changes use.

## What the account actually contains

Captured 2026-08-29 with `scripts/cutover/capture-aws.sh`, 20 of 20 sections
against the live account. Everything below is observed, not assumed.

**Region: `us-east-1`.** All seven buckets are there. This settles **A5** — the
region was an observation waiting to be made, not a choice.

**The corpus buckets already exist, under their GCS names:**
`ossf-scorecard-results`, `ossf-scorecard-cron-results`, `ossf-scorecard-data2`,
`ossf-scorecard-rawdata`, `ossf-scorecard-input-projects`,
`ossf-scorecard-cii-data`. One further bucket in the account is unrelated to the
corpus and out of scope here.

**The copy is running through AWS DataSync.** All 17 Secrets Manager entries are
`aws-datasync!loc-*` location secrets. Nothing else is in Secrets Manager, so
**none of the three application secrets exist yet** — `github`, `gitlab`, and
`fastly` all have to be created.

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
  api/                        # the serving plane; this change
    versions.tf               # required_version, provider constraints (A3)
    modules/
      state-backend/          # bootstrap only; see A4
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

There is **no `modules/storage/`**: the buckets are adopted, not created
(**A13**). There is no Kustomize overlay and no Kubernetes anywhere in the
serving plane (**A7**).

[sms]: https://opentofu.org/docs/language/modules/develop/structure/

**A3 — OpenTofu >= 1.10, pinned.** `use_lockfile` does not exist before 1.10:
the [v1.9 S3 backend docs][v19] document only `dynamodb_table` and state that
locking is disabled without it, while [v1.10][v110] adds `use_lockfile`. Latest
stable is 1.12.6. The constraint is declared in `versions.tf` and the exact
version in `.opentofu-version`, so the requirement surfaces as a readable error
at plan time rather than a confusing backend failure at `init`.

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

One caveat on the capture: every call against `ossf-scorecard-results` failed on
the TLS-interception error described in **A6**, so **nothing is yet known about
that bucket's configuration** — and it is the most important one. Re-capture it
from a clean network before relying on any property of it.

## Secrets

**A11 — Secrets Manager holds the values; OpenTofu creates the containers.**

The capture shows Secrets Manager currently holds only DataSync location
entries, so all three application secrets are new: `github` (`app_id`,
`app_key`, `installation_id`, `token`), `gitlab` (`auth_token`), and `fastly`
(`purge_token`). Three further secrets in the GKE cluster belong to the separate
`criticality-score` service and are not ours to move.

OpenTofu creates the secret *resources* and the IAM policy granting the task
role read access to only its own. It does **not** carry values: a value in a
variable is a value in state, and the state bucket would then be a credential
store with weaker controls than the service built for the purpose. Values are
loaded out-of-band and `ignore_changes` covers `secret_string`.

**The duplicated Fastly purge token collapses to one secret.** It exists in both
Secret Manager and the GKE cluster today, and a rotation that misses one copy
leaves half the system purging with a dead token — silently, because a failed
purge is indistinguishable from a cache that has not expired.

**GitLab is required, not optional.** `gitlab/auth_token` gates GitLab scanning
and there is a `cron/internal/data/gitlab-projects-releasetest.csv` lane. It
belongs to the batch plane, so it is created here only if the batch plane is not
going to own it — decide that before creating it, rather than creating it twice.

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

The measurement is cheap and the harness exists. Run
`scripts/api-conformance/conformance.sh` origin-to-origin between the gatewayed
production path and the gateway-less staging path; the diff is precisely ESPv2's
contribution. Whatever it shows is either folded into the application or
accepted deliberately — the point is that the cutover should not be the first
time anyone finds out.

Run it **origin-to-origin, not hostname-to-hostname**. Finding 6.3a established
that the two public hostnames cache separately under a year-long
`Surrogate-Control` and only one is purged, so a CDN-level comparison measures
cache vintage rather than behavior.

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
