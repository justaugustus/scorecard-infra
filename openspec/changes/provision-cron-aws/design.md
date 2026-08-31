# Design: the AWS batch scanning plane

Decision tags **E1**-**E9**. They are referenced from `tasks.md` and should be
cited in commit bodies, following the convention the other changes use.
(A/B/C/D/F/FF/W are taken by `provision-aws`, `configure-result-buckets`,
`migrate-batch-pipeline`, the original hybrid-server design,
`add-feature-flagging`, `add-upstream-fallback`, and `migrate-api`
respectively.)

## What the account already contains

Not captured fresh for this change — inherited from `provision-aws`'s
2026-08-29 account capture (`openspec/changes/archive/2026-08-30-provision-aws/design.md`),
which looked at the whole account, not just the serving plane. Task group 1
confirms nothing has changed since rather than re-running discovery from
zero.

**All six corpus buckets already exist**, under their GCS names, with AWS
DataSync actively writing to them: `ossf-scorecard-results`,
`ossf-scorecard-cron-results`, `ossf-scorecard-data2`,
`ossf-scorecard-rawdata`, `ossf-scorecard-input-projects`,
`ossf-scorecard-cii-data`. `provision-aws` adopted the first two; this change
adopts the rest.

**One SQS queue already exists**, `openssf-scorecard`, noted in that capture
only as "it belongs to the batch plane, which this change does not touch."
Nobody has looked closer since. Its name does not match
`cron/config/config.yaml`'s topic name (`scorecard-batch-requests`) — see
**E8**.

**The Elastic IP quota is 5, shared by both planes' NAT gateways.** Already
observed once; relevant here because it means there is no capacity reason to
avoid sharing a NAT Gateway with the serving plane (**E5**).

**No EKS cluster, no application IAM roles for the batch plane, and no SQS
consumer exist.** Confirmed by the same capture: all 21 IAM roles in the
account are service-linked or DataSync-created, and the only application
secrets are `deploy/cron/secrets`'s `scorecard/cron/{github,gitlab}`.

## Compute

**E1 — EKS, one cluster, two node groups.** Chosen over the serving plane's
ECS deliberately, not by default. Two reasons, both from you rather than
inferred: the two planes were split onto separate clusters so the serving and
batch migrations could proceed independently — you are the only one with
sustained bandwidth for this work, and coupling the two migrations' compute
would mean neither could move without the other — and so batch workloads
never contend with the serving tier for capacity on a shared cluster.
Secondarily, `cron/k8s/*.yaml` is already CronJob/Deployment/RBAC-shaped, so
EKS is the lower-rewrite landing zone; under a one-person time budget that is
not a minor consideration.

Two node groups, not one: a small system pool for the controller (a
once-a-week CronJob) and the GitHub token-pool server (a small, always-on
Deployment), and a separate worker pool sized to today's GKE baseline of 14
workers. Matches `provision-aws`'s **A8** pattern — start at the known
baseline, tune after verification, not autoscaling from day one.

## Queue

**E2 — SQS Standard + a DLQ replaces Pub/Sub.** Uncontested; the fan-out
shape (one controller publishing shards, many workers consuming) has no
serious competing AWS primitive.

**E3 — the visibility-heartbeat rework is the load-bearing code change, and
comes before anything else in `cron/internal/pubsub/`.**
`cron/internal/pubsub/subscriber_gcs.go` extends its ack deadline to 600
seconds and renews it before expiry — a scan can run long, and without that
extension a still-running scan becomes visible again and gets picked up by a
second worker. Plain SQS `ReceiveMessage` does not do this. The new
`subscriber_sqs.go` has to implement it explicitly: receive with long
polling, establish a visibility timeout, run a heartbeat that calls
`ChangeMessageVisibility` before it expires, stop the heartbeat on ack/nack/
shutdown/cancellation, and configure a DLQ with a finite max receive count.
SQS is at-least-once even with the heartbeat, so output writes stay
idempotent regardless. **No full-corpus run is scheduled until this is
proven** under a deliberately slow scan with a worker killed mid-run: the
message should stay invisible while healthy, become available after the
kill, complete on retry, and leave no inconsistent output.

Subscriber selection becomes URL-scheme-based
(`gcppubsub://` vs `awssqs://`) rather than the current always-GCP-unless-
emulated default, so the same binary can run against either backend during
the transition.

**E8 — the pre-existing `openssf-scorecard` queue is investigated, not
assumed.** It might be a leftover from an earlier, abandoned attempt at this
same migration; a placeholder someone created intending to wire it up later;
or already correctly shaped and simply undocumented. Task group 1 finds out
before **E2** decides whether to adopt it or create a fresh queue + DLQ pair.

## Storage

**E6 — the six existing corpus buckets are adopted, not created.** Same
reasoning as `provision-aws`'s **A13**: they already exist and hold live,
DataSync-replicated data, so OpenTofu references them as data sources and
grants IAM access. Declaring them as managed resources would put the corpus
one `-target` mistake or one deleted block away from deletion, and OpenTofu
cannot distinguish "this should not exist" from "someone removed the block
that declared it."

**E7 — three new buckets, genuinely created, for testing without touching
production data.** Cross-referencing `cron/config/config.yaml` against the
six adopted buckets: a normal scan cycle writes to exactly three —
`result-data-bucket-url` (`data2`, shard/completion state),
`api-results-bucket-url` (`cron-results`), and `raw-result-data-bucket-url`
(`rawdata`). `input-projects` is read-only from the controller's side and
`cii-data` is written only by the separate, lower-frequency CII cronjob, so
neither gets a test counterpart yet.

`cron-results` is the most dangerous of the three to test against directly:
it is the *same bucket* `deploy/api` reads as `SCORECARD_CRON_RESULTS_BUCKET_URL`,
the live API's fallback for every repository without a self-published
result. A test write there can surface in a real `api.scorecard.dev`
response — not a corpus-integrity problem, a live-serving-correctness one.

Proposed names, to confirm during scaffolding: `ossf-scorecard-cron-results-test`,
`ossf-scorecard-data2-test`, `ossf-scorecard-rawdata-test` — the production
names with a `-test` suffix, so a bucket list is self-explanatory without a
lookup table. These three *are* declared as OpenTofu resources; **E6**'s
"don't declare what you didn't create" reasoning does not apply to buckets
this change creates.

## Network

**E5 — shares `deploy/api`'s VPC and NAT Gateway; no second one is
provisioned.** Cost-driven and confirmed directly: a dedicated VPC's NAT
Gateway runs roughly $32-35/month plus data processing, on top of the
~$110-145/month already budgeted for the serving plane in `provision-aws`,
for isolation the cluster split (**E1**) already provides at the compute
layer. The account capture's own note — "the Elastic IP quota is 5, which is
the ceiling both planes share for NAT gateways" — shows this was anticipated
before either plane had a NAT Gateway at all.

The batch cluster gets its own private subnets and route tables inside the
existing VPC; nothing about EKS vs ECS or worker vs task isolation depends on
the network layer being separate too. Accepted tradeoffs, stated rather than
hidden: an edit to `deploy/api/modules/network` (a new AZ, a CIDR resize) now
affects both planes' root modules, reintroducing a sliver of the
cross-change coordination the cluster split was meant to avoid; and a NAT
Gateway outage or a connection-tracking limit hit during simultaneous heavy
egress from both planes would be a shared-fate event. Neither outweighs the
cost of a second NAT Gateway today. Revisit if either ever causes real
friction — this is reversible, not structural.

## Workload deployment

**E4 — Kubernetes manifests, not Terraform resources.** OpenTofu's job here
stops at the AWS-side resources: the cluster, the node groups, Pod Identity
role-to-service-account mappings, the queue, and bucket IAM policies. A
GitHub Actions workflow applies `cron/k8s/{controller,worker,cii,auth}.yaml`
directly via `kubectl`, after `aws eks update-kubeconfig` and OIDC
authentication — the same deployment shape the manifests already assume.
Converting them into `kubernetes_manifest` Terraform resources would trade
away the one property that made EKS the right call under a one-person time
budget (**E1**) for no compensating benefit: the manifests port with registry
references and node-selectors adjusted, not rewritten.

The controller's existing narrow RBAC (`Role`/`RoleBinding` scoped to `get`,
`patch` on the `scorecard-batch-worker` Deployment, used to trigger a rollout
after publishing a shard) needs no AWS-side equivalent — it is a Kubernetes
API permission, and EKS's control plane is still a real Kubernetes API. This
is one of the concrete costs an ECS choice would have paid: ECS has no
equivalent primitive, and reproducing the same restart trigger would mean a
new `ecs:UpdateService` IAM grant plus rewriting the controller's restart
call entirely, not just retargeting it.

## State

**E9 — the existing state backend is reused, not re-bootstrapped.**
`deploy/cron/secrets` already writes into the same S3 state bucket
`deploy/api/bootstrap` created, under the key `cron/secrets/terraform.tfstate`.
This change adds sibling keys (e.g. `cron/production/terraform.tfstate`) in
the same bucket. No new `deploy/cron/bootstrap` root module.

The new root is `deploy/cron/production/`, flat under `deploy/cron/` —
matching `deploy/cron/secrets`'s existing layout, not `deploy/api`'s nested
`environments/{staging,production}/`. That nesting exists for the serving
plane because staging and production need materially different bucket
access (staging is read-only against production's data); no such split
exists for the batch plane, and inventing one to match a different plane's
directory shape would be cosmetic. If a batch staging/rehearsal tier is ever
needed, `deploy/cron/production/` becomes the sibling of a
`deploy/cron/staging/` at that point, not retroactively nested under an
`environments/` directory neither one needs.

## Verification approach

Mirrors `provision-aws`'s gate structure, adapted for a plane with no
internet-facing traffic to shadow-test:

1. Publish a small, explicit repository inventory to the queue.
2. Confirm the message stays invisible while a worker is actively scanning it
   (**E3**'s heartbeat).
3. Kill that worker mid-scan. Confirm the message becomes visible again,
   a second worker picks it up, and the result completes on retry.
4. Confirm no inconsistent output landed in the **test** buckets (**E7**) from
   the killed attempt — a partial write, a duplicate, or a result tagged with
   the wrong commit would all be signals to fix before pointing at production
   data.
5. Only after that passes does reconfiguring against the six adopted
   production buckets, and scheduling the production `cron/k8s/*.yaml`
   schedules, become the next change's job to gate and execute.

## What this change does not decide

- **The BigQuery/warehouse replacement.** Explicitly deferred; see the
  proposal's non-goals.
- **The release-test tier's fate.** Whether it still runs at all is
  unconfirmed; porting it is not this change's job.
- **`:stable` tag promotion.** Unrelated to where compute runs.
- **Node group autoscaling.** Start at the known baseline (**E1**); tuning is
  a follow-on once there is a real signal to tune against.
