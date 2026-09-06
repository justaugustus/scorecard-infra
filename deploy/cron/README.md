# AWS batch scanning plane

OpenTofu for the batch scanning pipeline — the `cron/` tree, which scans 1M+ repositories weekly and writes results to object storage.

This deploys **only the batch plane**. The results API is a separate deployment under `deploy/api/` and shares nothing here but the account. See `openspec/changes/provision-cron-aws/` for the proposal, the decision record (**E1**–**E9**), and the task list.

## Requirements

- **OpenTofu >= 1.10.** Not optional: state locking uses `use_lockfile`, native S3 conditional writes, which does not exist before 1.10 — v1.9 documents only `dynamodb_table` and disables locking silently without it. Each root module declares the constraint; `../.opentofu-version` pins the exact version for both planes, and version managers of the `tenv`/`tofuenv` family find it by searching upward from wherever you are standing.
- AWS credentials for the target account.
- **Run off any TLS-intercepting network.** A VPN that re-signs certificates makes the AWS SDK fail with `SSL validation failed ... self-signed certificate in certificate chain`, which reads like a permissions error and is not one.
- **kubectl** access to the batch cluster (EKS).

## Layout

```text
modules/
  cluster/     EKS control plane and node groups; IAM roles; workload associations
  queue/       SQS queue and DLQ; message configuration
secrets/       Secrets Manager secrets for SCM and CDN credentials
production/    root module, own state key
config-aws.yaml   working config overlay; points at production buckets and inventories
```

Each root module is independent — `modules/cluster`, `modules/queue`, and `production/` each carry their own `terraform` block and provider configuration. Terraform and OpenTofu do not inherit those from a parent directory, so there is no tree-wide `versions.tf` and the duplication is required rather than sloppy.

`production/` is **the root module with the live state key.** Workload manifests sit under `cron/k8s/` alongside the batch code.

## Apply order

The batch plane depends on the serving plane's VPC and S3 gateway endpoint. **Apply the serving plane (`deploy/api/production`) before the batch plane.** Within the batch plane:

1. `deploy/cron/secrets/` first — creates Secrets Manager secrets for SCM and CDN credentials.
2. `deploy/cron/production/` — creates the network, queue, cluster, and Pod Identity associations.

```sh
cd deploy/cron/production

# The bucket is chosen once at bootstrap, so it is supplied at init
tofu init -backend-config=bucket=<state bucket>

tofu plan
tofu apply
```

**IAM before ConfigMap.** When switching between test and production buckets (task 9.7), the IAM policy grants must be applied **before** the ConfigMap pointing workers at new buckets. Reversed, workers point at buckets they cannot write, and every shard fails `AccessDenied`.

## Workload configuration

The batch controller publishes to SQS; workers consume from SQS and read/write to S3 buckets. The queue and bucket URLs embed AWS account identifiers and must be supplied at deployment time, not committed to public Git.

### ConfigMap: `scorecard-queue`

Created from the queue's actual URLs by Terraform output or by hand:

```sh
kubectl create configmap scorecard-queue \
  --from-literal=request-topic-url=awssqs://<queue-url> \
  --from-literal=request-subscription-url=awssqs://<queue-url> \
  --dry-run=client -o yaml | kubectl apply -f -
```

The two URLs are the same (SQS has no separate topic/subscription, so the queue plays both roles).

### ConfigMap: `scorecard-config`

The working overlay, created from `deploy/cron/config-aws.yaml`:

```sh
kubectl create configmap scorecard-config \
  --from-file=config.yaml=deploy/cron/config-aws.yaml \
  --dry-run=client -o yaml | kubectl apply -f -
```

Contains four writable bucket URLs (production) and one read-only input bucket. **Replace the `-test` suffix if rolling back to test buckets.**

### Kubernetes Secrets

Three secrets, created by hand (until the Secrets Store CSI driver lands):

```sh
kubectl create secret generic scorecard-secrets \
  --from-literal=github-app-key=<GitHub App PEM> \
  --from-literal=gitlab-token=<GitLab PAT> \
  --from-literal=fastly-purge-token=<Fastly API token> \
  --dry-run=client -o yaml | kubectl apply -f -
```

Secrets Manager holds the actual values in `deploy/cron/secrets/main.tf`; the Pod Identity roles grant access; the CSI driver would translate them to Kubernetes Secrets. Until then, create them by hand and update the policy when they rotate (they are optional at runtime — workers log "CDN purging disabled" if unset).

## Application configuration

`modules/cluster/main.tf` defines the four Pod Identity IAM roles (controller, worker, CII worker, GitHub server) and their policies. This table maps the IAM grants to the code that exercises them:

| Workload | Permissions | Purpose |
| --- | --- | --- |
| **controller** | `sqs:SendMessage`, `GetQueueAttributes` on the main queue; `s3:PutObject` on `data2` and `rawdata`; read-only on `input-projects` | Publishes shards; writes completion markers |
| **worker** | `sqs:ReceiveMessage`, `DeleteMessage`, `ChangeMessageVisibility`, `GetQueueAttributes` on the main queue; read/write on `data2`, `rawdata`, `cron-results`; read-only on `cii-data` | Consumes shards; writes results; reads CII corpus |
| **CII worker** | read/write on `cii-data` only | Refreshes the CII corpus weekly |
| **github-server** | read-only on the github secret | Serves GitHub App metadata |

The IAM policy grants production buckets alongside `-test` counterparts, which means the plane switches between them via ConfigMap re-apply without IAM changes landing mid-flight. Rollback is config-only.

## Verification and deployment

The batch plane is verified by:

1. **Validation** (`make test`, `tofu validate`) before commit.
2. **Smoke test** (`run-sample-inventory.sh`) — 3 sample repos written to production with all grants verified.
3. **Failure scenarios** (`kill-worker-mid-scan.sh`, `trigger-dlq.sh`) — message redelivery, DLQ, and denial boundaries confirmed.
4. **IAM denials** (`verify-iam-denials.sh`) — runtime `AccessDenied` confirmed for out-of-scope bucket/queue access.

**To deploy a change:**

```sh
# 1. On cutover-N branch, validate and commit
make build && make test && make lint
(cd deploy/cron && tofu fmt -check -recursive . && tofu validate)
git commit -s

# 2. IAM first (if bucket grants changed)
(cd deploy/cron/production && tofu apply)

# 3. ConfigMap second (if config changed)
kubectl create configmap scorecard-config \
  --from-file=config.yaml=deploy/cron/config-aws.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

# 4. Workers last (to pick up both config and image)
kubectl rollout restart deployment/scorecard-batch-worker
kubectl rollout status deployment/scorecard-batch-worker --timeout=10m
```

See `openspec/changes/provision-cron-aws/` for the full task runbook (9.7) and the provision design.

## What this does not manage

- **The adopted production buckets.** Six buckets (`data2`, `rawdata`, `cron-results`, `cii-data`, `results`, `input-projects`) already exist and hold data, so they are referenced as `data` sources and never declared as resources (**E7**). Declaring them would put the corpus one `tofu destroy`, one `-target` mistake, or one deleted block away from deletion — and OpenTofu cannot tell "this should not exist" from "someone removed the block that declared it."
- **Secret values.** The secret *resources* and their access policies are managed; the values are loaded out-of-band and `ignore_changes` covers them. A value passed to OpenTofu is a value written to state, which would make the state bucket a credential store with weaker controls than the service built for it.
- **Workload schedules.** The batch controller runs as a CronJob on the Kubernetes cluster; the schedule is defined in `cron/k8s/controller.yaml`, not here. The `scorecard-cii-worker` CronJob refreshes the CII corpus weekly; both schedules are part of the Kubernetes deployment, not the infrastructure provisioning.
- **Credential delivery to workloads.** Pod Identity grants access; workloads read credentials as Kubernetes Secrets. Until the Secrets Store CSI driver is deployed, those Secrets are created by hand. The CSI driver would translate Secrets Manager into those Secrets automatically.
