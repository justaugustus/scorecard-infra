# Applying changes to the batch scanning cluster

These manifests target the **EKS** cluster built by `deploy/cron/production`.
They previously targeted the GKE `openssf` cluster, which was turned down on
2026-08-31 along with the rest of the GCP project.

There is no automation yet — changes are applied by hand with `kubectl`.
Automating this is task 8.1 of
`openspec/changes/provision-cron-aws/`.

The `*.release.yaml` tier and the `transfer*.yaml` jobs are **not** ported and
still name GCP registries and buckets. That is deliberate (task 7.3): the
release-test environment's status is unresolved, and BigQuery transfer is out
of scope for this change.

Before committing, check the YAML with [yamllint](https://yamllint.readthedocs.io):

```console
yamllint -d relaxed .
```

## Connecting `kubectl`

```console
aws eks update-kubeconfig --name scorecard-batch --region us-east-1
kubectl config current-context
```

The cluster uses EKS access entries (`authentication_mode = "API"`), not the
legacy `aws-auth` ConfigMap, so access is granted through IAM rather than by
editing anything in the cluster.

## Applying a manifest

```console
kubectl apply -f FILENAME
```

Everything runs in the `default` namespace, which is what the Pod Identity
associations in `deploy/cron/modules/cluster` are scoped to.

## The two ConfigMaps

### `scorecard-config` — the application config

On AWS this comes from `deploy/cron/config-aws.yaml`, **not** from
`cron/config/config.yaml`. The latter still names `gs://` buckets and a
`gcppubsub://` topic. Note the `--from-file` rename: every workload mounts it
at `/etc/scorecard/config.yaml`.

```console
kubectl create configmap scorecard-config \
  --from-file=config.yaml=../../deploy/cron/config-aws.yaml \
  -o yaml --dry-run=client | kubectl apply -f -
```

Its buckets are the four **test** buckets. Task 9.7 repoints them at the
production corpus, and only after verification passes.

### `scorecard-queue` — the SQS URL

Kept out of `config-aws.yaml` because an SQS queue URL contains the AWS
account ID and this repository is public. Read it from Terraform rather than
pasting a literal:

```console
QUEUE_URL="$(cd ../../deploy/cron/production && tofu output -raw queue_url)"
SQS_URL="awssqs://${QUEUE_URL#https://}?region=us-east-1"

kubectl create configmap scorecard-queue \
  --from-literal=request-topic-url="$SQS_URL" \
  --from-literal=request-subscription-url="$SQS_URL" \
  -o yaml --dry-run=client | kubectl apply -f -
```

Both keys hold the same value. SQS has no separate topic and subscription, so
the controller publishes to and the worker consumes from one queue.

## Secrets

`worker.yaml` and `auth.yaml` read three Kubernetes Secrets — `github`,
`gitlab` and `fastly` — and the worker also mounts `github` as a file for the
GitHub App key. A missing Secret leaves the pod in
`CreateContainerConfigError`, so these must exist before the workloads are
applied.

The values live in AWS Secrets Manager under `scorecard/cron/*`, created by
`deploy/cron/secrets`. **Nothing yet syncs them into the cluster** — the
Pod Identity roles grant `secretsmanager:GetSecretValue`, but that is a
permission with no caller until the Secrets Store CSI driver lands (task 7.2's
remaining half). Until then, create them by hand:

```console
aws secretsmanager get-secret-value --secret-id scorecard/cron/github \
  --query SecretString --output text > /tmp/github.json

kubectl create secret generic github \
  --from-literal=app_id="$(jq -r .app_id /tmp/github.json)" \
  --from-literal=installation_id="$(jq -r .installation_id /tmp/github.json)" \
  --from-literal=token="$(jq -r .token /tmp/github.json)" \
  --from-file=app_key=<(jq -r .app_key /tmp/github.json) \
  -o yaml --dry-run=client | kubectl apply -f -

shred -u /tmp/github.json
```

Repeat for `gitlab` (key `auth_token`) and `fastly` (key `purge_token`). The
key names are not arbitrary — they are the `secretKeyRef` keys in
`worker.yaml` and `auth.yaml`.

Once the CSI driver lands this whole section goes away, which is the point of
doing it.

## Images

Manifests reference `ghcr.io/ossf/scorecard-*:main`, published by
`.github/workflows/publish-cron-images.yml` on every merge to `main`.

`:main` is mutable. It is used here because it is the only tag that currently
resolves — `:stable`, which these manifests named on GKE, has never been
published from this repository, and `:latest` appears only once a `cron/v*`
release tag is cut. Task 8.2 has CI deploy by digest instead; what promotes an
image to a stable tag is still open as `migrate-batch-pipeline` task 4.4.
