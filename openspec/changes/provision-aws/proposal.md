# Proposal: Provision the AWS serving environment in OpenTofu

## Why

This repository's stated purpose is provider-agnostic hosted infrastructure, and
the serving path has been methodically decoupled from one cloud to serve it.
`configure-result-buckets` replaced the API's compile-time `gs://` constants with
`SCORECARD_RESULTS_BUCKET_URL` and `SCORECARD_CRON_RESULTS_BUCKET_URL` and linked
`s3blob` alongside `gcsblob`. `publish-container-images` moved every image to
`ghcr.io`, a registry with no cloud affinity. The Sigstore publish path depends
on Fulcio and Rekor rather than on a hosting provider, and the CDN is Fastly,
which already sits outside any cloud.

What remains is that **the neutrality claim has never been executed.** The API
can address an `s3://` URL because the driver is linked and the configuration is
an environment variable — but it has never been run against real S3, by anyone.
A capability exercised against exactly one backend is a design intention, not a
property, and the only way to convert it is to stand up a second target and put
the existing conformance harness against it.

There is a second reason, independent of the first: **none of the current
deployment is reproducible from version control.** No `.tf` exists anywhere in
this repository. `docs/research/data-infra.md` lists "deployment is not
reproducible or digest-pinned today" among the design's known correctness debt,
and the cutover captures in `migrate-api` group 6 are the evidence of what that
costs — a shell script and three attempts were needed just to *read* the running
configuration, and two of those attempts chased resources that did not exist.

The corpus copy makes this time-sensitive rather than merely overdue. A capture
of the AWS account on 2026-08-29 found the result buckets already present in
`us-east-1` with DataSync actively writing to them, and **no compute
infrastructure of any kind** — no cluster, no load balancer, no certificate, no
VPC beyond the default. Data is arriving somewhere that has nothing to serve it,
and everything that will serve it is still unwritten.

## What Changes

- **Add `deploy/api/`**, holding the serving environment as OpenTofu modules with
  per-environment root configurations for `staging` and `production` under
  separate state keys. `deploy/cron/` is reserved for the batch plane and is not
  part of this change.
- **Run the API on ECS Fargate behind an ALB.** The serving and batch planes are
  deployed separately: the API takes internet traffic and the batch pipeline does
  not, so they get different blast radii, and without a shared cluster there is
  nothing left to justify Kubernetes for one stateless container. No EKS, no
  Kustomize, no cluster to operate.
- **Adopt the corpus buckets; do not create them.** They exist and hold data.
  OpenTofu references them as data sources and grants the task role access.
- **Provision what is genuinely absent**: a purpose-built VPC with private
  subnets and an S3 gateway endpoint, the three application secrets, an ACM
  certificate and ALB as the Fastly-verifiable origin, and a GitHub Actions OIDC
  role for digest-pinned deployment.
- **Keep `scripts/cutover/capture-aws.sh` current.** It has run once and the run
  corrected three of this change's assumptions and exposed three defects in the
  script; both sets are folded in.
- **Measure what the ESPv2 gateway contributes** before concluding it contributes
  nothing, by running the conformance harness origin-to-origin between the
  gatewayed production path and the already-gateway-less staging path.

### Explicitly out of scope

- **Production traffic.** This change ends at an AWS-backed *staging* origin that
  passes conformance. Moving the production Fastly backend is `migrate-api`
  group 6.
- **The batch plane.** Its queue already exists in the account; its S3 and SQS
  drivers do not — `grep s3blob cron/` returns nothing. `migrate-batch-pipeline`
  owns that, and it gets its own environment under `deploy/cron/`.
- **Bucket configuration changes.** The capture shows versioning disabled and no
  lifecycle rules on the corpus buckets. Both are worth addressing and neither
  should arrive as a side effect of provisioning a web service.
- **ECR.** Earlier planning mapped GCR to ECR; superseded. Images publish to
  `ghcr.io`, and no container registry is created.
- **BigQuery and any replacement.** AGENTS.md's cloud-agnostic rules state "No
  BigQuery," and `cron/internal/bq` is coupled to GCS and BigQuery directly.
- **DNS hosting.** Both zones are delegated to Netlify DNS — confirmed from the
  AWS side by the account holding zero Route 53 zones. Records this change needs
  are emitted as outputs and created there.

## Impact

- **Affected code:** new `deploy/api/` tree, and `scripts/cutover/capture-aws.sh`.
  No change to `api/`, `cron/`, `internal/`, or `cmd/`. Both frozen trees stay
  frozen; this change is what their configuration seams were built for, and
  exercising a seam is not editing it.
- **Affected specs:** adds `infrastructure-as-code` and `aws-serving`.
- **New toolchain dependency:** OpenTofu >= 1.10, recorded as a
  `required_version` constraint rather than left for `tofu init` to discover.
- **Reversible by construction.** Nothing here enters a request path until a
  Fastly backend field changes, and this change does not touch that field. The
  failure mode of abandoning it is some unused AWS resources.
- **Cost**, approximate and to be confirmed against current list prices: roughly
  $110-145/month for the serving environment — Fargate tasks, ALB, one NAT
  gateway, secrets, and logs. Fargate bills per running task-second rather than
  per request, so this does not reintroduce the per-request metering the
  migration exists to escape.

## Open questions carried into implementation

1. **Which plane owns the GitLab secret.** It gates batch scanning, not serving.
   Creating it in both places is worse than deciding once.
2. **Task count and size.** Two tasks is an availability floor, not a
   measurement. The number that should replace it is origin request rate — cache
   misses per second, not public traffic — and conformance has to run before
   there is anything to measure.
3. **Versioning is off on every corpus bucket, including the one the API reads.**
   Out of scope here, and worth its own decision soon: `latest` is an
   unconditional overwrite, so a delayed run can regress it, and there is
   currently nothing to recover from when it does.
