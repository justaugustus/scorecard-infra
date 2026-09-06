# Tasks: Provision the AWS batch scanning plane in OpenTofu

Decision tags **E1**–**E9** are defined in `design.md`.

## 1. Discovery

Run 2026-08-31 with `scripts/cutover/capture-aws.sh` and a new sibling,
`scripts/cutover/capture-aws-batch.sh`, which covers the DataSync, SQS
attribute, CloudWatch and route-table sections the first script has none of.

- [x] 1.1 Confirm the six corpus buckets (`ossf-scorecard-results`,
      `ossf-scorecard-cron-results`, `ossf-scorecard-data2`,
      `ossf-scorecard-rawdata`, `ossf-scorecard-input-projects`,
      `ossf-scorecard-cii-data`) are still present and still being written by
      DataSync, as `provision-aws` 1.4 found on 2026-08-29. Re-run
      `scripts/cutover/capture-aws.sh` rather than trust a five-day-old
      capture.
      **All six present in `us-east-1`. Versioning off, lifecycle absent,
      bucket policy absent, encryption and public-access-block present on
      every one — confirming E7's premise rather than assuming it. Newest
      date partition in `data2` and `rawdata` is `2026.08.24/`, matching the
      frozen corpus exactly. DataSync is still running but degrading; see the
      note under group 3.**
- [x] 1.2 Investigate the `openssf-scorecard` SQS queue (**E8**): its
      attributes, redrive policy (if any), and message count. Determine
      whether it is a leftover from an earlier attempt, an unwired
      placeholder, or already correctly shaped. Decide adopt-vs-create before
      **E2** is implemented.
      **Unwired placeholder, and not an old one: created 2026-08-26 by the
      account root user via the console, then tuned 2026-08-28
      (`VisibilityTimeout` 30→3600, `ReceiveMessageWaitTimeSeconds` 0→20).
      CloudWatch reports 0 sent / 0 received / 0 deleted on every day since
      creation — it has never carried a message. No redrive policy, no tags,
      SSE off, and its name matches neither `cron/config/config.yaml`'s topic
      (`scorecard-batch-requests`) nor its subscription. See 4.1.**
- [x] 1.3 Confirm no EKS cluster, no batch-plane application IAM roles, and no
      SQS consumer already exist, as `provision-aws` 1.7 found for the
      serving plane's equivalents.
      **Confirmed on all three. Zero EKS clusters. Of 29 IAM roles, none is a
      batch-plane application role — 8 service-linked, 8 Application
      Migration, 7 `AWSDataSyncS3BucketAccess-*`, 6 serving-plane roles
      created 2026-08-29. No consumer: zero Lambda event source mappings, and
      the queue's own receive count is zero for its whole lifetime.**
- [x] 1.4 Confirm the Elastic IP quota (5, shared by both planes per
      `provision-aws` 1.7) has enough headroom for the batch cluster's node
      groups' egress, given the serving plane's NAT Gateway is being shared
      (**E5**) rather than doubled.
      **Three of five free. `describe-addresses` returns six, which reads as a
      breach of a quota that has never been raised; it is not. Four are
      `RequesterManaged: true` under `amazon-elb`, on the two ALBs'
      interfaces, and service-managed addresses do not count against
      `L-0263D0A3`. Only the two NAT Gateway EIPs are self-allocated, which
      `deploy/`'s single `aws_eip` declaration corroborates. E5 stands on cost
      alone — there is no capacity argument for it.**

## 2. Toolchain and scaffolding

- [x] 2.1 Create `deploy/cron/modules/{cluster,queue}/` and
      `deploy/cron/production/`, each with its own `terraform` block pinning
      `required_version >= 1.10` (matching `deploy/api` and
      `deploy/cron/secrets`).
      **`modules/cluster` and `modules/queue` are scaffolding only — a
      `terraform` block and nothing else, since their resources are groups 5
      and 4 respectively. `deploy/cron/production/` got its full network and
      test-bucket content directly (see group 3), since 2.1's module list has
      no `network` module — the batch plane attaches to an already-live VPC
      it doesn't own, rather than building one, so those resources sit in the
      root rather than a reusable module.**
- [x] 2.2 `deploy/cron/production/`'s backend: S3, same state bucket
      `deploy/cron/secrets` uses, key `cron/production/terraform.tfstate`,
      `use_lockfile = true` (**E9**). No new bootstrap.
- [x] 2.3 Tag convention matches `deploy/cron/secrets`: `Project = "scorecard"`,
      `Component = "cron"`, `ManagedBy = "opentofu"`,
      `Source = "ossf/scorecard-infra//deploy/cron/production"`.

## 3. Network

- [x] 3.1 Determine how `deploy/cron/production/` reaches `deploy/api`'s VPC
      — a `terraform_remote_state` data source against `deploy/api`'s state,
      or a small shared network module both roots call. Decide before writing
      subnets (**E5**).
      **Decided with Stephen 2026-08-31: `terraform_remote_state` against
      `deploy/api/production`'s state (`vpc_id`, subnet CIDRs, endpoint ID) —
      read-only, one direction. For the S3 gateway endpoint's associations,
      chose the association-resource split over design.md's original
      `additional_route_table_ids`-input draft: `deploy/api/modules/network`
      gets a one-time edit to stop setting `route_table_ids` inline on
      `aws_vpc_endpoint.s3` and declare `aws_vpc_endpoint_route_table_association`
      for its own two existing private route tables instead; `deploy/cron/production`
      declares its own association resources against the same endpoint ID.
      One-time edit to `deploy/api`, not a recurring one every time cron's
      route tables change. See design.md's E5 section for the full reasoning.**
- [x] 3.2 Add private subnets and route tables for the batch cluster inside
      that VPC, routed through the existing NAT Gateway. No new NAT Gateway,
      no new Elastic IP.
      **Written in `deploy/cron/production/main.tf`: two private subnets
      (`var.private_subnet_cidrs`, defaulting to the two `/20`s design.md
      picked), one route table per AZ, each routed to
      `data.terraform_remote_state.api_production.outputs.nat_gateway_ids[0]`
      — index `[0]` because `deploy/api/modules/network`'s
      `single_nat_gateway` defaults true, so production has exactly one.
      AZs come from that same remote-state read (`outputs.availability_zones`,
      added in 3.3) rather than a second `aws_availability_zones` lookup, so
      the two roots cannot resolve to different zones.
      **Applied 2026-08-31: two private subnets (`us-east-1a`/`10.21.160.0/20`,
      `us-east-1b`/`10.21.176.0/20`) with one route table each, both routed
      through the shared NAT Gateway. `tofu apply`: 22 added, 0 changed, 0
      destroyed, matching the plan exactly.**
- [x] 3.3 Edit `deploy/api/modules/network`: replace `aws_vpc_endpoint.s3`'s
      inline `route_table_ids` with `aws_vpc_endpoint_route_table_association`
      resources for its own two private route tables (additive at the AWS API
      level — same associations, different resource shape). Verify with a
      full `tofu plan` (no `-target`) against `deploy/api/production` before
      applying; this touches live serving-plane state. Then declare matching
      `aws_vpc_endpoint_route_table_association` resources in
      `deploy/cron/production` for the batch route tables from 3.2, against
      the endpoint ID read via 3.1's remote-state data source.
      **Both sides written. `deploy/api/modules/network/main.tf`'s
      `aws_vpc_endpoint.s3` no longer sets `route_table_ids`; two
      `aws_vpc_endpoint_route_table_association` resources cover its existing
      two private route tables. `deploy/api/modules/network/outputs.tf`
      gained `nat_gateway_ids`; `deploy/api/environments/production/outputs.tf`
      gained `vpc_id`, `nat_gateway_ids`, `s3_endpoint_id`, and
      `availability_zones` for 3.1/3.2 to read. `deploy/cron/production`
      declares its own two association resources against the same endpoint
      ID. `tofu validate` passes for both `environments/production` and
      `environments/staging` (staging also calls `modules/network`, so the
      edit had to stay safe for both — it is, since the resulting AWS-side
      associations are unchanged, only the resource that manages them
      changed).
      **Both sides applied 2026-08-31, by Stephen, in order. `deploy/api/production`
      first: 3 added (its own two `aws_vpc_endpoint_route_table_association`
      resources, plus an unrelated ECS task definition replacement from an
      intentional image digest bump bundled into the same apply), 1 changed,
      1 destroyed. Then `deploy/cron/production` (3.2/3.4's apply): its own
      two association resources against the same shared S3 gateway endpoint
      — four associations on one endpoint total, no conflict, confirming the
      association-resource split's whole premise under a real apply.**
- [x] 3.4 Create the test buckets (**E7**): `ossf-scorecard-cron-results-test`,
      `ossf-scorecard-data2-test`, `ossf-scorecard-rawdata-test`, and
      `ossf-scorecard-cii-data-test`. Private,
      versioning on (unlike the adopted production buckets, which have it
      off — no reason to inherit that gap in something created fresh).
      **Written in `deploy/cron/production/main.tf` via `var.test_buckets`,
      names as proposed. Versioning, AES256 SSE, and a full public-access
      block on each, matching `deploy/api/modules/state-backend`'s bucket
      style minus the state bucket's `prevent_destroy` and
      deny-insecure-transport policy — not warranted for buckets this
      disposable. Applied 2026-08-31, all three created empty and versioned
      as planned. `cii-data` was added to `var.test_buckets` afterwards, when
      group 5 gave the CII worker a role and it had nowhere safe to write —
      see the amended **E7**; it is the one bucket in this task not yet
      applied.**

## 4. Queue

- [x] 4.1 Resolve **E8**: adopt `openssf-scorecard` or create a new queue,
      per task 1.2's finding.
      **Decided in `design.md`'s E8 section (task 1.2): create fresh, leave
      the existing placeholder queue alone. `deploy/cron/modules/queue`
      creates `scorecard-batch-requests` and its DLQ, named to match
      `cron/config/config.yaml`'s existing Pub/Sub topic name — SQS has no
      separate topic/subscription concept, so this one queue plays both
      roles once group 6 lands.**
- [x] 4.2 Provision the SQS Standard queue and its DLQ (**E2**), with a
      finite `maxReceiveCount` redrive policy.
      **`deploy/cron/modules/queue/main.tf`: `aws_sqs_queue.this` with a
      `redrive_policy` pointing at `aws_sqs_queue.dlq`,
      `maxReceiveCount = var.max_receive_count` (default 5 — not a carried
      value, since E8's placeholder queue never had a redrive policy at
      all). Wired into `deploy/cron/production` as `module "queue"`. Applied
      2026-08-31: 2 added, 0 changed, 0 destroyed, matching the plan
      exactly.**
      **Amended after review (Kusari AWS-0096): both queues now state
      `sqs_managed_sse_enabled = true` rather than inheriting SQS's service
      default. Re-applied 2026-09-05 and the plan answered the question it
      existed to answer — "No changes", so the default had already encrypted
      both queues and the finding was a false positive. The line stays
      anyway: a stated intent survives a future provider version that starts
      sending the attribute, where an inherited one does not. Same refresh
      covered every resource in this root and found no drift against groups
      3, 4 and 5.**
- [x] 4.3 Size the visibility timeout default to comfortably exceed a typical
      scan's duration — this is a starting value the heartbeat (**E3**)
      extends, not the sole protection against redelivery.
      **`visibility_timeout_seconds` defaults to 3600, `receive_wait_time_seconds`
      to 20 — both carried from E8's finding: the pre-existing unwired
      `openssf-scorecard` queue was tuned to exactly these values on
      2026-08-28 by whoever understood this workload, even though that
      queue itself was not reused.**

## 5. Cluster

- [x] 5.1 EKS cluster, one control plane, in the subnets from group 3.
      **`deploy/cron/modules/cluster`: `aws_eks_cluster.this` in group 3's
      two private subnets, `access_config { authentication_mode = "API",
      bootstrap_cluster_creator_admin_permissions = true }` — modern
      EKS access management, no legacy aws-auth ConfigMap. The bootstrap
      flag matters: under API auth mode nobody has cluster access by
      default, not even the applying principal, so without it whoever runs
      `tofu apply` would create a cluster they cannot immediately `kubectl`
      into. Version pinned to **1.36**, the newest on EKS standard support
      as of 2026-08-31 — an earlier draft pinned 1.31, whose standard
      support ended in 2025 and which would have billed the control plane at
      the extended-support rate, roughly six times standard and more per
      month than the second NAT Gateway **E5** declined to buy. Endpoint
      reachable privately (node traffic stays off the shared NAT Gateway)
      and publicly (group 8's CI has no stable egress range to allowlist, so
      IAM is the control, not the network). Control-plane `api`/`audit`/
      `authenticator` logs enabled into a declared log group with finite
      retention, rather than EKS's silent never-expire default.
      **Applied 2026-08-31: 28 added, 0 changed, 0 destroyed across the whole
      root, matching the plan exactly. The control plane took 10m6s of a
      13m30s apply; the `cii-data` test bucket from 3.4 and every IAM role
      landed in the first second.**
- [x] 5.2 System node group: sized for the controller CronJob (bursts to one
      pod once a week) and the `scorecard-github-server` Deployment
      (always-on, small).
      **`aws_eks_node_group.system`, defaulted small (starting instance
      type/count, tunable — A8), two nodes rather than one because CoreDNS
      runs two replicas and wants two nodes to spread across. Untainted, so
      cluster DaemonSets and add-on components land here. One IAM node role
      shared by both node groups; per-workload access lives in Pod Identity,
      not node-level IAM.**
- [x] 5.3 Worker node group: sized for 14 workers (**E1**'s baseline),
      matching the current GKE `scorecard-batch-worker` Deployment replica
      count.
      **`aws_eks_node_group.worker`. `cron/k8s/worker.yaml` sets no
      `resources.requests/limits`, so there is no hard sizing signal in the
      manifest to size nodes against — instance type/count are a defensible
      starting guess (A8), not derived from anything. 14 is the Deployment
      replica count group 7 ports; this task is about how many *nodes* host
      those pods, a separate number. Sized so each of the 14 pods gets
      roughly a vCPU and 4 GiB, with ephemeral storage well above EKS's
      20 GiB default — worker pods clone repositories to local disk, several
      per node, and a full volume evicts pods rather than failing one scan.
      This is the plane's largest cost lever; revisit against real
      utilisation after group 9. Carries a `NoSchedule` taint keyed on the
      same `scorecard.dev/pool` label both pools set: without it the two-pool
      split is decorative, since nothing would stop 14 scanning pods landing
      on the system node. Group 7.1 owes `worker.yaml` the matching
      toleration. Both node groups applied 2026-08-31 — worker in 1m37s,
      system in 2m8s, in parallel after the control plane.**
- [x] 5.4 Pod Identity associations: a role per workload
      (controller/worker/CII worker/github-server), each scoped to exactly
      what that workload needs — the queue actions its role requires, the
      buckets it reads/writes, and (worker only) `deploy/cron/secrets`'
      `read_policy_json` output.
      **Four roles, four `aws_eks_pod_identity_association` resources — one
      per `cron/k8s/*.yaml` workload. An earlier draft wrote three and
      omitted the CII worker, which would have left `cii.yaml` falling back
      to the node role and unable to write anything at all.
      controller: `sqs:SendMessage`/`GetQueueAttributes` on the main queue,
      read-only on the adopted `input-projects` bucket (no test counterpart
      — E7), ~~read/write on the `data2` test bucket (shard/completion
      state)~~ **corrected 2026-09-06: read/write on both `data2` and
      `rawdata` (shard/completion state) — see 9.9's correction below, this
      task's original scoping missed a write `main.go` has always made**,
      and **no secrets policy** — the same draft attached one, but there is
      no `os.Getenv` anywhere in `cron/internal/controller/` and
      `controller.yaml` carries no `secretKeyRef`, so it reads no credential.
      worker:
      `sqs:ReceiveMessage`/`DeleteMessage`/`ChangeMessageVisibility`/`GetQueueAttributes`
      on the main queue, read/write on the `cron-results`/`data2`/`rawdata`
      test buckets ~~(not `cii-data-test` — that belongs to the CII worker
      alone)~~ **corrected 2026-09-06: plus read-only (`s3:GetObject`,
      `s3:ListBucket`) on `cii-data-test`. "Belongs to the CII worker alone"
      is right about writes and wrong about reads. Every scan builds
      `clients.BlobCIIBestPracticesClient` over
      `config.GetCIIDataBucketURL()` (`cron/internal/worker/main.go`) and the
      CII-Best-Practices check calls `bucket.Exists` then `bucket.ReadAll` on
      `<repo>/result.json` in that bucket. Observed live 2026-09-06 via
      `scripts/verification/run-sample-inventory.sh`: 3 of 3 repos, every run,
      `check CII-Best-Practices has a runtime error: ... code=PermissionDenied
      ... HeadObject ... 403`. It does not crash the worker — a per-check
      runtime error degrades and moves on, like the unrelated
      Branch-Protection token limitation in the same log — so it would have
      reached the full inventory as silently missing scores rather than as a
      failure. Root cause is the same shape as 9.9's `rawdata` finding: a
      design-time assumption about what a workload needs, wrong about what the
      code does at runtime. GCP never surfaced it because no `cron/k8s`
      manifest set `serviceAccountName` before 7.1, so all four workloads
      shared the namespace's `default` ServiceAccount and one identity; the
      worker held the CII job's access by accident. Splitting into four roles
      was right, dropping this read was not. Granted as its own read-only
      statement (`ReadCIIData`) rather than a fourth `worker_bucket_keys`
      entry, which would have added `PutObject`/`DeleteObject` over the CII
      corpus**, plus the combined secrets read policy. CII worker: the
      narrowest of the four — read/write on `cii-data-test` and nothing
      else, since `cron/internal/cii/main.go` fetches the OpenSSF Best
      Practices pages over plain HTTP and writes one bucket, with no queue
      and no credential. `scorecard-github-server`: no queue, no bucket — a
      policy scoped to *only* the github secret, narrower than the worker's
      combined policy, since it has no business reading the gitlab or fastly
      credentials. None of the `deploy/api` roles from `provision-aws`, the
      DLQ, or any of the six adopted production buckets appear in any policy
      this task wrote — that absence is what 5.6 verifies.
      **A credential the manifests need that no root created:**
      `worker.yaml` sets `FASTLY_PURGE_TOKEN`, and `deploy/cron/secrets`
      created only github and gitlab. Added `fastly` there rather than
      reading `deploy/api`'s across roots — two planes purging the same CDN
      are two consumers, and one token each means either can be rotated
      without taking the other down. It is optional at runtime:
      `cron/internal/worker/main.go` logs "CDN purging disabled" and
      continues with a no-op client when the variable is unset, which is the
      correct behaviour during group 9 anyway. `deploy/cron/secrets` applied
      2026-08-31 (one secret added, nothing else touched) and the value
      loaded out of band; the worker's policy now covers all three
      credentials.
      **Load-bearing findings for group 7.1, not yet acted on there:** none
      of `cron/k8s/{controller,worker,cii,auth}.yaml` sets
      `serviceAccountName` today, so all four implicitly share the `default`
      ServiceAccount. Pod Identity associates one role per (cluster,
      namespace, service account) tuple — four workloads on one shared
      ServiceAccount could only ever get one shared role, handing each
      workload every other one's access. Every association above names a
      workload-specific ServiceAccount (`scorecard-batch-controller`,
      `scorecard-batch-worker`, `scorecard-cii-worker`,
      `scorecard-github-server`) that does not exist in the manifests yet;
      7.1 must create each and set it, and `controller.yaml`'s `RoleBinding`
      subject needs the same update, away from `default`. Separately, the
      IAM grant is not a delivery mechanism: the manifests read credentials
      as *Kubernetes* Secrets (`secretKeyRef`, and a mounted file for the
      GitHub App key), and nothing yet translates Secrets Manager into those
      — see 7.2. All four roles, their policies and their associations
      applied 2026-08-31; the `eks-pod-identity-agent` addon installed after
      both node groups were up, per the ordering added for it.**
- [x] 5.5 The controller's Kubernetes RBAC (`Role`/`RoleBinding` scoped to
      `get`, `patch` on the `scorecard-batch-worker` Deployment) needs no
      AWS-side IAM equivalent — verify it applies to EKS's control plane
      unchanged (**E4**).
      **Confirmed structurally: it's a Kubernetes API permission, and
      EKS's control plane is a real Kubernetes API regardless of
      authentication mode — no AWS-side resource in this module stands in
      for it. It does need a content change in group 7.1 (not an AWS-side
      one): the `RoleBinding`'s subject is currently `ServiceAccount:
      default`, which 5.4's finding above already requires changing to the
      controller's dedicated ServiceAccount for Pod Identity to work at
      all — so this task and 5.4's finding land on the same manifest edit.**
- [x] 5.6 Verify denial, not only permission: confirm each role is refused
      access to the other planes' buckets/secrets and to the production
      corpus buckets from a role scoped to the test buckets, not only that
      its own grants work.
      **Partially satisfied by construction, not yet by a runtime test:**
      no policy in `deploy/cron/modules/cluster` names the DLQ, any
      `deploy/api` resource, or any of the six adopted production bucket
      ARNs — reviewed directly against the module's source, not inferred.
      That is necessary but not sufficient: a policy that never grants
      access is not the same as a runtime `AccessDenied` observed from a
      running pod. This task's AWS-side half is done; the behavioral half is
      **task 9.9**, added for the purpose. An earlier draft deferred it to
      "9.4/9.6, which cover adjacent ground" — they do not: 9.4 checks
      output consistency and 9.6 runs the CII cycle, and neither would fail
      if a role turned out to hold a grant it should not.

## 6. Code

- [x] 6.1 Link `gocloud.dev/blob/s3blob` in `cron/data/blob.go` alongside
      `fileblob` and `gcsblob`.
      **One blank import. `blob.OpenBucket` is already a scheme registry, so
      `s3://` needs nothing else, and the S3 SDK was already present for the
      serving plane.**
- [x] 6.2 Implement `cron/internal/pubsub/subscriber_sqs.go` (**E3**): long
      polling `ReceiveMessage`, an initial visibility timeout, a heartbeat
      goroutine calling `ChangeMessageVisibility` before expiry, `Ack` →
      `DeleteMessage`, `Nack` → `ChangeMessageVisibility` to zero, heartbeat
      stopped on ack/nack/shutdown/cancellation.
      **Built on gocloud's driver rather than a hand-rolled SQS client. The
      renewal rides on `As()`, gocloud's documented escape hatch for provider
      behaviour the portable API does not cover, and `Ack`/`Nack` delegate to
      the driver — its `Nack` already issues `ChangeMessageVisibility` with a
      timeout of zero, so the task's requirement is met without writing it.**
      **Renewal is 600s with a 60s grace, matching `subscriber_gcs.go`'s
      existing `ackDeadlineExtensionInSec`/`gracePeriodInSec` rather than
      introducing a second, differently-tuned pair. Cost is not a factor at
      any plausible interval: `shard-size: 10` makes 1.3M repos/week roughly
      130k messages, only long shards renew at all, and even assuming every
      message renewed three times it is under a dollar a month. Idle long
      polling by 14 replicas costs more, and 4.3 already fixed that.**
      **Three deliberate departures from `subscriber_gcs.go`: a failed
      renewal is logged and retried rather than `log.Fatal` (losing one
      message to redelivery beats losing every scan in flight on the worker);
      heartbeat shutdown is idempotent, so `Close` after `Ack` cannot panic on
      a second channel close; and a transient `Receive` error backs off and
      retries instead of returning `(nil, nil)`, which `cron/worker` reads as
      "stop" and turns into an exit 0 — a momentary SQS error would otherwise
      cycle every replica and report it as a clean shutdown. `AccessDenied`
      and a missing queue still return, since retrying those forever stops
      scanning just as effectively and says nothing about why.**
      **The subscription is opened through an explicitly configured
      `awssnssqs.URLOpener`, not the one the driver registers on the default
      mux: the default receive batcher prefetches ten messages while the
      worker processes one, leaving nine buffered with nothing renewing them.
      That option is reachable only through the exported opener, not through
      URL query parameters. One message may still be buffered ahead, which is
      safe — the driver never sets `VisibilityTimeout` on `ReceiveMessage`, so
      it inherits the queue's own hour-long default from 4.3.**
- [x] 6.3 Add a matching SQS publisher alongside the existing GCP one in
      `cron/internal/pubsub/publisher.go`.
      **A blank import of `gocloud.dev/pubsub/awssnssqs`, and nothing else.
      `CreatePublisher` already routes through `pubsub.OpenTopic`, and
      `publisherImpl`'s batching and error accounting never knew which cloud
      it was talking to. The publish side has no heartbeat to add, so
      gocloud's abstraction is sufficient here in a way it is not for the
      subscriber.**
- [x] 6.4 Switch subscriber/publisher selection to URL scheme
      (`gcppubsub://` vs `awssqs://`) rather than the current
      always-GCP-unless-emulated default.
      **`CreateSubscriber` switches on the parsed scheme using the drivers'
      own exported constants (`awssnssqs.SQSScheme`, `gcppubsub.Scheme`), so a
      URL this accepts is a URL those drivers accept, and an unrecognised one
      returns a wrapped `errUnsupportedScheme` instead of defaulting to a
      backend nobody asked for. The `PUBSUB_EMULATOR_HOST` check moved into
      `createGCPSubscriber` unchanged — it now selects between the two GCP
      implementations rather than between GCP and everything else.**
      **The publisher needs no equivalent: `pubsub.OpenTopic` on the default
      mux already dispatches on scheme off the blank imports.**
- [x] 6.5 Unit tests: heartbeat renewal, ack/nack/DLQ paths, a deliberately
      slow consumer that outlives one visibility window without losing the
      message, malformed URLs, and the existing GCP paths unchanged.
      **`subscriber_sqs_test.go` and `subscriber_test.go`. The slow-consumer
      case was verified by deleting the heartbeat and confirming it goes red
      with the message it was written to emit, rather than trusting a green
      assertion. Also covered: renewal stops on each of ack/nack/close/context
      cancel, `Close` after `Ack` does not panic, a failed renewal is retried
      rather than fatal, transient receive errors retry while permanent ones
      return, a missing receipt handle fails loudly instead of running on
      without a heartbeat, and scheme dispatch including unknown, absent and
      malformed URLs.**
      **Messages come from gocloud's in-memory driver, because
      `pubsub.Message`'s ack hooks are unexported and a zero value panics on
      `Ack`. No GCP construction is tested: both GCP constructors reach for
      real credentials, and a test needing a GCP project proves nothing about
      routing. That path is a verbatim move and stays covered by the existing
      `TestSubscriber`.**
- [x] 6.6 `go build ./...`, `go test ./... -race`, `golangci-lint run ./...`
      clean with both subscriber implementations compiled in.
      **All three clean, plus `go mod tidy` with no resulting diff. `sqs`,
      `aws-sdk-go-v2` and `smithy-go` move from indirect to direct; `sns`
      joins as indirect via the driver.**
- [x] 6.7 Prove the driver's runtime shapes against a live queue, not a mock.
      Added while closing 6.5: `As()` conversions either match the driver or
      they do not, and a mock agrees with whatever the test hands it, so the
      one thing group 6 could not self-verify was whether
      `msg.As(&sqstypes.Message)` returns anything at runtime. If it does not,
      `SynchronousPull` fails on the first message and every worker dies.
      **`subscriber_sqs_integration_test.go`, behind a build tag and skipped
      unless `SCORECARD_SQS_TEST_QUEUE_URL` is set, so the default suite
      implies no credentials. Publishes one marked message through
      `CreatePublisher`, receives it through `CreateSubscriber`, holds it past
      several renewal windows, acks it, and reads the queue's own depth to
      confirm the delete rather than inferring it.**
      **Run against `scorecard-batch-requests` 2026-09-05: receipt handle
      recovered (412 bytes), three renewals accepted by SQS, heartbeat stopped
      on ack, queue drained to zero visible and zero in flight.**
      **It failed on its first run, which is the argument for having written
      it. `SynchronousPull` returns a nil message only once the context is
      cancelled, and a nil message is the only thing that breaks
      `cron/worker`'s loop, so `Close` was reachable by no route other than a
      dead context — and it passed that context to `Shutdown`, failing every
      graceful shutdown by construction. The loud symptom was a non-zero exit
      on SIGTERM; the quiet one was that `Shutdown` flushes gocloud's pending
      ack batch, so the just-finished shard's ack was dropped and SQS
      redelivered it. Fourteen duplicate scans per rollout, and it would have
      read as ordinary at-least-once noise rather than a defect. `Close` now
      detaches the context and takes ten seconds of its own, inside
      Kubernetes' default termination grace period.**
      **The existing mock could not have caught it — its `Shutdown` ignored
      the context. The regression test uses one that fails on a dead context
      like the real subscription, and was confirmed red against the previous
      behaviour before being kept.**
      **This harness is what tasks 9.2–9.5 need against the test buckets, so
      it is a file rather than a throwaway script. It does not close 9.2:
      that wants the real worker running in-cluster, not a published marker.**

## 7. Workload manifests

- [x] 7.1 Port `cron/k8s/controller.yaml`, `worker.yaml`, `cii.yaml`,
      `auth.yaml` for EKS: the new `awssqs://` topic/subscription URLs and
      the AWS config overlay's bucket URLs (`s3://...`). ~~Registry references
      already point at `ghcr.io` — no image changes.~~ **Wrong, corrected
      2026-09-05: all four named `gcr.io/openssf/*:stable`. The registry moves
      to `ghcr.io/ossf`, and the tag moves off `:stable`, which
      `publish-cron-images.yml` has never published — checked against the
      registry directly, where `:main` returns 200 while `:latest` and
      `:stable` both 404. `:main` is the checked-in reference until task 8.2
      deploys by digest; this does not settle `migrate-batch-pipeline` 4.4.**
      Three things group 5
      requires here, none of them optional:
      - A distinct `ServiceAccount` object per manifest, with
        `serviceAccountName` set to match the cluster module's
        `workload_service_accounts` output — `scorecard-batch-controller`,
        `scorecard-batch-worker`, `scorecard-cii-worker`,
        `scorecard-github-server`. All four currently run as `default`, and
        Pod Identity cannot give four workloads on one ServiceAccount four
        different roles.
      - `controller.yaml`'s `RoleBinding` subject repointed from
        `ServiceAccount: default` to the controller's own.
      - `nodeSelector` on all four for the `scorecard.dev/pool` label, plus
        a toleration on `worker.yaml` for the worker pool's `NoSchedule`
        taint (**E1**). Without the toleration the worker Deployment stays
        `Pending` — deliberately loud.
      **All three done, plus a fourth this task did not anticipate:
      `auth.yaml`'s Service pinned `clusterIP: 10.4.4.210`, an address from
      the GKE service range. EKS allocates from its own and refuses anything
      outside it, so that Service would have been rejected outright. The
      literal was referenced nowhere, so the pin is simply gone.**
      **Pool placement: the worker takes the worker pool and its toleration;
      the controller, CII job and token server take the system pool. The CII
      job fetches from the Best Practices API once a month rather than
      cloning repositories, so it has no claim on a scanning node.**
      **Hardening, which the port inherited rather than introduced: no
      manifest declared a `securityContext` and none but the controller
      declared `resources`. Every pod now sets `runAsNonRoot` and
      `RuntimeDefault` seccomp; every container drops all capabilities,
      refuses privilege escalation, and runs read-only. The images already
      resolve to non-root UIDs (65532 for the distroless bases, 1001 for the
      kubectl sidecar), both numeric, which is what lets the kubelet verify
      `runAsNonRoot` instead of refusing to start on an unresolvable name —
      but these are enforced regardless of what an image claims about
      itself.**
      **A read-only root needs a writable `/tmp`: the scanning path extracts
      repositories via `os.MkdirTemp("", ...)` and `kubectl` caches discovery
      under `$HOME`. Both get an `emptyDir`, and the sidecar's `HOME` is
      repointed at it. This is the one part of the port that cannot be
      verified before a deploy — a write path outside `/tmp` would surface as
      a failing scan, not a failing apply.**
      **Worker resource bounds are sized to the cluster module's own
      m5.xlarge reasoning: 14 pods requesting 28Gi across three nodes, with a
      memory ceiling so one hostile repository pressures its own pod rather
      than the node. No CPU limit — throttling a scan makes it slow, not
      safe. Starting values per A8.**
      **The `awssqs://` URLs are not in the manifests. They arrive as
      `SCORECARD_REQUEST_TOPIC_URL` (controller) and
      `SCORECARD_REQUEST_SUBSCRIPTION_URL` (worker) from a `scorecard-queue`
      ConfigMap — a queue URL embeds the AWS account ID, and this repository
      is public. Same reasoning that scrubbed live resource IDs from the
      proposal.**
- [ ] 7.2 Add an AWS config overlay (e.g. `deploy/cron/config-aws.yaml` or a
      ConfigMap generated from one) analogous to `cron/config/config.yaml`,
      pointing at the test buckets (**E7**) until group 9 passes, and leaving
      BigQuery fields disabled/absent — this change does not deploy the
      transfer jobs.
      **Also owns the credential delivery mechanism, which group 5 grants
      but does not build.** `worker.yaml` and `auth.yaml` read credentials as
      Kubernetes Secrets (`secretKeyRef`, and a mounted file for the GitHub
      App key); nothing in the cluster translates a Secrets Manager secret
      into one. The Secrets Store CSI driver with the AWS provider is the
      obvious candidate and authenticates via exactly the Pod Identity 5.4
      set up. Until this lands, the `secretsmanager:GetSecretValue` grants
      are a permission with no caller.
      **Overlay done, credential delivery still open — this task stays
      unchecked on the second half.**
      **`deploy/cron/config-aws.yaml` carries the four test buckets and the
      adopted read-only `input-projects`, and omits BigQuery entirely rather
      than naming a dataset nothing writes. Two findings came from reading the
      consumers instead of assuming: `metric-exporter` cannot be blank, since
      `cron/monitoring`'s `GetExporter` treats empty as an error and
      `cron/internal/worker` calls it during start-up — so "metrics off"
      spelled as an empty string is a `CrashLoopBackOff`. It is `printer`, the
      in-tree no-backend exporter. `project-id` survives for the same reason,
      read only on paths that do not run here.**
      **`cron/config/config_aws_test.go` parses the overlay through this
      package's own parser, so drift fails in CI rather than in a pod. It also
      asserts every writable bucket is a `-test` one, which makes task 9.7
      edit a test to reach production rather than quietly changing a URL.**
      **Credential delivery is deferred to its own change: the CSI driver
      needs an EKS addon in `deploy/cron/modules/cluster`, `SecretProviderClass`
      objects, and another human-run apply, which does not belong in a
      manifest port. `cron/k8s/README.md` documents creating the three Secrets
      by hand in the meantime — the same thing GKE did — with the exact key
      names `worker.yaml` and `auth.yaml` expect, since a missing key surfaces
      only as `CreateContainerConfigError`.**
- [x] 7.3 Confirm the `*.release.yaml` tier and `transfer*.yaml` are **not**
      ported — out of scope (proposal non-goals).
      **Confirmed untouched: `controller.release.yaml`, `worker.release.yaml`,
      `webhook.release.yaml`, `transfer.yaml`, `transfer-raw.yaml`,
      `transfer.release.yaml`, `transfer.release-raw.yaml`. They still name
      `gcr.io` images and `gs://` buckets, which is the correct state for a
      tier this change does not deploy — `migrate-batch-pipeline` 4.4a records
      that `webhook.release.yaml` names an image nothing has ever built, and
      whether that environment still exists is unanswered. Recorded in
      `cron/k8s/README.md` so the mixed tree is legible rather than looking
      like a half-finished port.**
- [ ] 7.4 Convert `cron/k8s/` to a Kustomize base plus overlays. 7.1 edited the
      manifests in place, which was right while there is exactly one cluster:
      GKE is gone, so a second copy would only drift. That stops being true as
      soon as the `*.release.yaml` tier is revived or a staging cluster
      appears, and the seams are already visible — the four AWS workloads
      differ from their `.release` counterparts by image tag, bucket set and
      queue, which is precisely what an overlay expresses. Deferred rather
      than done now because it introduces Kustomize to a repository with none,
      and group 8's deploy step would have to learn it in the same change.

## 8. CI/deploy

- [ ] 8.1 GitHub Actions workflow: OIDC to AWS (constrained to this
      repository and a protected environment, matching `provision-aws`'s
      **A9**-adjacent pattern for the serving plane's deploy role),
      `aws eks update-kubeconfig`, `kubectl apply -f` the manifests from
      group 7.
- [ ] 8.2 Deploy by digest where the workflow can (image references already
      come from `publish-cron-images.yml`'s digest output); document where it
      still can't (e.g. `:stable`, per the open `migrate-batch-pipeline` 4.4
      question) rather than silently accepting a mutable tag.
- [ ] 8.3 `actionlint` and `zizmor` clean on the new workflow.
- [ ] 8.4 Evaluate closing the cluster's public API endpoint, once 8.1 has
      shown what the deploy path actually needs. Raised in review against
      task 5.1's open `public_access_cidrs`, and recorded here rather than
      fixed there because it is not a value change: allowlisting
      GitHub-hosted runners is impossible, not merely unpleasant.
      `api.github.com/meta` advertised **7,251** CIDR blocks under `actions`
      on 2026-09-05 against an EKS quota of **40** that AWS documents as not
      adjustable. Closing the endpoint therefore means moving the caller
      inside the VPC — self-hosted runners in group 3's private subnets, or
      a bastion — which trades one open-but-IAM-guarded endpoint for runner
      infrastructure this plane would then own and patch. Decide with 8.1's
      workflow in hand; a human `kubectl` path (5.1's bootstrap admin) has to
      survive whatever is chosen.

## 9. Verification

- [x] 9.1 Publish a small, explicit repository inventory to the queue,
      pointed at the test buckets (**E7**) via the config overlay from 7.2.
      **A 3-repo sample (`github.com/ossf/scorecard`,
      `github.com/ossf/scorecard-infra`, `github.com/octocat/Hello-World`)
      run as a one-off Job cloned from the CronJob's own pod spec, args
      repointed at a ConfigMap-mounted sample file instead of the two
      baked-in inventory CSVs — same ServiceAccount, so it exercises the
      real controller Pod Identity role. First run published successfully
      but then panicked writing the raw-bucket completion marker
      (`AccessDenied`, see 9.9's correction and 5.4). Re-run after the fix
      completed cleanly: publish, worker consumption, and both bucket
      writes (`data2-test`, `rawdata-test`) all succeeded, with identical
      per-repo scores across both runs.**
      **This also gave the first positive (not merely absence-of-crash)
      evidence for two of the three unknowns flagged after the initial
      deploy: `readOnlyRootFilesystem` (three real repos cloned and scanned
      successfully) and Pod Identity/SQS reachability (an observed
      publish → receive → ack → delete cycle, not just idle pods holding
      steady). The third, the CII worker's association, is 9.6.**
      **Formalized as `scripts/verification/run-sample-inventory.sh` so
      re-verifying after a future change doesn't mean re-deriving these
      commands — it clones the live CronJob's pod spec rather than
      duplicating it, so it tracks whatever `cron/k8s/controller.yaml`
      actually has deployed.**
- [x] 9.2 Confirm the message stays invisible while a worker is actively
      scanning it (**E3**'s heartbeat holding the visibility timeout open).
      **Closed by existing evidence rather than a new artifact:
      `subscriber_sqs_integration_test.go`'s `TestSQSRoundTrip` (task 6.7)
      already proves the heartbeat mechanism against a real queue, and
      9.1's live run is a real worker, in-cluster, observably holding a
      message not-visible during a real scan — together, mechanism and
      real-worker evidence, which is what 6.7's own note said 9.2 still
      needed beyond the mechanism alone.**
      **A same-shaped new integration test (`TestAbandonedReceiveRedelivers`,
      built and reverted this session — see 9.3's note) was going to add a
      second data point here, but 9.3's finding made it moot before it
      could run.**
- [x] 9.3 Kill that worker mid-scan (e.g. `kubectl delete pod`). Confirm the
      message becomes visible again after the timeout, a second worker picks
      it up, and the result completes on retry.
      **Attempted as a Go integration test first (extending
      `subscriber_sqs_integration_test.go` with the same
      tuned-visibility-field trick 6.7 uses), on the theory that a manual
      redelivery test would be as fast against the real queue as 6.7's
      round trip is. It hung for 5 minutes instead: the live plane now runs
      14 real worker pods continuously long-polling
      `scorecard-batch-requests`, and a test's single subscriber cannot win
      a receive race against 14 always-on competitors for its own marker
      message. Confirmed via worker pod logs — `"Received message from
      subscription"` at timestamps matching the test's publish. This
      wasn't true when 6.7 was written; the cluster didn't exist yet.
      Fixing it properly needs a dedicated, isolated test queue (mirroring
      **E7**'s dedicated test buckets), which is new infrastructure and a
      new `tofu apply` — out of scope for this pass. The two new tests were
      reverted; `subscriber_sqs_integration_test.go` is back to its
      committed state.**
      **Closed for real instead with `scripts/verification/kill-worker-mid-scan.sh`
      against a live shard. First attempt's pod-selection query used a label
      (`app=scorecard-batch-worker`) that doesn't exist on these pods —
      `cron/k8s/worker.yaml`'s Deployment labels them
      `app.kubernetes.io/name=worker` — so it found nothing and the script
      correctly refused to guess rather than kill an arbitrary pod. Fixed
      and re-run: identified `scorecard-batch-worker-77cdb75c48-rpbzv`
      holding the shard and force-deleted it
      (`--grace-period=0 --force`) mid-scan. The Deployment replaced it
      immediately (back to 14/14 running within seconds), and the shard
      still completed — confirmed via S3, not just queue depth, since a 1s
      polling granularity missed the exact redelivery moment: `data2-test`
      and `rawdata-test` both show a completed shard for this run's
      timestamp, with all 3 sample repos present, no duplicates, same
      scores and commits as the unkilled runs.**
- [x] 9.4 Confirm no inconsistent output landed in the test buckets from the
      killed attempt — no partial write, no duplicate, nothing tagged with
      the wrong commit.
      **`scripts/verification/check-bucket-consistency.sh` run against the
      exact shard 9.3 killed and retried: `data2-test`/`rawdata-test` each
      hold exactly one shard file for that timestamp (no partial pair, no
      duplicate), all six `cron-results-test` per-repo exports (clean +
      commit-SHA copies) present for the 3 sample repos, and the shard's 3
      result lines match the unkilled runs' scores and commits exactly —
      `github.com/ossf/scorecard` 8.9, `github.com/ossf/scorecard-infra`
      6.2, `github.com/octocat/Hello-World` 3.1, same commits each time.
      No inconsistent output from the killed attempt.**
- [x] 9.5 Confirm the DLQ receives a message that permanently fails (e.g. a
      deliberately malformed shard) after `maxReceiveCount` retries, rather
      than looping forever.
      **`scripts/verification/trigger-dlq.sh` publishes one well-formed but
      permanently unscannable message directly to the real queue (an
      `example.invalid` repo URL — not malformed JSON, which would crash
      the whole worker process before its ack/nack logic ever ran, per the
      script's header) and lets the real worker fleet fail it naturally.
      Run for real 2026-09-06: reached the DLQ after exactly 5 deliveries
      (the queue's configured `maxReceiveCount`), confirmed by content
      match, then cleaned up. Real worker contention — the problem for
      9.3 — is not a problem here, since any of the 14 workers failing the
      message repeatedly is the real path, not something to route around.**
- [x] 9.6 Confirm the CII worker completes one cycle against
      `ossf-scorecard-cii-data-test`. The earlier "or document why it was
      left pointed at the production bucket" escape is gone: group 5 gave
      the CII worker its own Pod Identity role, the amended **E7** gave it
      its own test bucket, and its role is scoped to that bucket alone, so
      writing production is no longer something it can do.
      **Triggered as a one-off Job via `scripts/verification/run-cii-cycle.sh`
      (`kubectl create job --from=cronjob/scorecard-cii-worker`) rather than
      waiting for its own real schedule (`0 20 * * 0`, ~11 hours out at the
      time). Completed cleanly; `ossf-scorecard-cii-data-test` holds 11,618
      `result.json` objects afterward, confirming both a full real cycle
      against the live bestpractices.dev API and the CII worker's Pod
      Identity association — the third of the three unknowns 9.1 flagged
      as unverified after the initial deploy, now closed.**
- [ ] 9.7 Only after 9.1–9.6 and 9.9 pass: repoint the config overlay at the six
      production buckets and the real `projects.csv`/`gitlab-projects.csv`
      inventories, but do **not** enable the production `cron/k8s/*.yaml`
      schedules yet — that activation, and the community notice question it
      may raise, is this change's last task before closeout, not an
      automatic consequence of verification passing.
- [x] 9.8 `tofu fmt -check -recursive -diff deploy/cron/` and
      `tofu validate` (per-root, `-backend=false`) clean.
      **Both clean. `tofu validate` run against `deploy/cron/production` and
      `deploy/cron/secrets` with the real backend already initialized (from
      the rawdata IAM fix's apply) rather than `-backend=false` — a strictly
      stronger check, since it also confirms backend connectivity, not just
      config syntax.**
- [x] 9.9 Verify denial at runtime, closing task 5.6's behavioral half. From
      a pod running under each role, confirm an actual `AccessDenied` — not
      merely an absent grant — for: the worker writing any of the six
      adopted production buckets; the worker calling `ReceiveMessage` on the
      DLQ; the CII worker touching any bucket but `cii-data-test`; the
      controller writing ~~`cron-results-test` or `rawdata-test`~~
      **corrected 2026-09-06: `cron-results-test` only** (see below); and
      any of the four reading a `deploy/api` secret. Run this **before**
      9.7 repoints anything at production, since 9.7 legitimately widens
      the first of those grants and the check stops being meaningful after
      it.
      **`rawdata-test` dropped from this list 2026-09-06: 9.1 hit a real
      `AccessDenied` there, and it turned out not to be a false grant to
      guard against.** `cron/internal/controller/main.go` has always
      written a `.shard_metadata` completion marker to the raw bucket
      whenever `raw-result-data-bucket-url` is configured — traced to
      ossf/scorecard#2451 (2022), which built this to be optional for other
      consumers of this same controller binary (the PR names
      `criticality-score`), not something specific to Scorecard's own
      pipeline or this migration. `cron/config/config.yaml` has set that
      value on GCP the whole time, so the controller's GCP production role
      must have granted this same write for years without incident — 5.4's
      data2-only scoping was an incomplete audit of `main.go`'s actual
      writes, not a considered exclusion, and is corrected there.
      `deploy/cron/modules/cluster`'s `ShardCompletionState` statement
      grants both buckets to the controller as a result.
      **Run 2026-09-06 via `scripts/verification/verify-iam-denials.sh`: all
      five boundaries confirmed `AccessDenied` from a throwaway
      `aws-cli` pod under each workload's real ServiceAccount — the worker
      writing `ossf-scorecard-data2` (production), the worker calling
      `ReceiveMessage` on the DLQ, the CII worker writing `data2-test`, the
      controller writing `cron-results-test`, and the worker reading
      `scorecard/production/fastly`. First run caught two bugs in the
      script itself, not the infrastructure — DLQ URL resolution used
      `basename`, which splits on `/`, against a colon-separated SQS ARN,
      and two sequential `kubectl wait` calls (Succeeded, then Failed)
      wasted a full 60s per check waiting on the condition that never
      happens, since every check here is expected to fail. Fixed in a
      follow-up commit; re-run confirmed all five in under a minute.**

## 10. Documentation

- [ ] 10.1 Write `deploy/cron/README.md`, mirroring `deploy/api/README.md`'s
      shape: requirements, layout, apply order, an **Application
      configuration** table (env vars / config overlay keys mapped to the
      code that reads them, matching what `deploy/api/README.md`'s
      equivalent section now has), and what this deployment does not manage.
- [ ] 10.2 Update `AGENTS.md`'s `cron/` row and batch-pipeline section: GCP
      production is gone, AWS compute exists, pointing at
      `deploy/cron/README.md` for the runbook.
- [ ] 10.3 Update the root `README.md`'s batch scanning pipeline section with
      the AWS deployment path.

## 11. Closeout

- [ ] 11.1 `openspec validate provision-cron-aws --strict` passes.
- [ ] 11.2 Verify every success criterion in the proposal and design is met.
- [ ] 11.3 Archive the change once implemented and merged.
