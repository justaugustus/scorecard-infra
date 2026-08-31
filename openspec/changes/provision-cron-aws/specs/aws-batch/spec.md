# aws-batch: Added Requirements

## ADDED Requirements

### Requirement: The batch plane runs on its own cluster, independent of serving

The batch scanning plane's compute SHALL run on infrastructure independent of
the serving plane's, with separate identities, so that neither plane's
migration, deployment, or failure depends on the other's compute.

#### Scenario: One plane is provisioned, changed, or destroyed

- **WHEN** either plane's compute is provisioned, changed, or destroyed
- **THEN** the other SHALL continue serving or scanning unaffected

#### Scenario: Batch load cannot starve serving, and vice versa

- **WHEN** the batch workload scales out to its full worker count
- **THEN** it SHALL NOT consume compute capacity the serving workload requires,
  because the two run on separate clusters with no shared node pool

#### Scenario: A workload assumes an identity

- **WHEN** a batch workload obtains cloud credentials
- **THEN** the identity SHALL belong to that workload alone, and SHALL NOT be
  assumable by the serving plane or by another batch workload's role

### Requirement: A still-running scan cannot be double-worked

The queue SHALL keep an in-progress unit of work invisible to other consumers
for at least as long as that work is actually running, and SHALL make it
visible again only after the assigned worker fails to complete or acknowledge
it.

#### Scenario: A scan is running normally

- **WHEN** a worker is actively scanning a shard
- **THEN** the queue SHALL NOT redeliver that shard to another worker

#### Scenario: A worker is killed mid-scan

- **WHEN** a worker terminates before acknowledging its shard
- **THEN** the shard SHALL become visible again after the queue's visibility
  timeout elapses, and a different worker SHALL be able to claim it

#### Scenario: A shard fails permanently

- **WHEN** a shard fails processing repeatedly past a configured retry limit
- **THEN** it SHALL land in a dead-letter queue rather than being retried
  indefinitely or silently dropped

### Requirement: Corpus buckets are adopted, never declared as resources

The batch plane's OpenTofu SHALL reference the existing, already-populated
corpus buckets as data sources and SHALL NOT declare them as managed
resources, so that no OpenTofu operation on this deployment can delete or
recreate them.

#### Scenario: The batch cluster reads or writes a corpus bucket

- **WHEN** a bucket already holds production data and this deployment reads or
  writes it
- **THEN** OpenTofu SHALL reference it as a data source

#### Scenario: A destroy or a `-target` mistake is run

- **WHEN** this deployment is destroyed, or applied with a narrow `-target`
- **THEN** no corpus bucket SHALL be deleted, because none is declared as a
  resource this deployment owns

### Requirement: Verification runs against isolated buckets before production data is touched

Before the batch pipeline is pointed at the production corpus buckets, its
correctness SHALL be demonstrated against buckets created for that purpose
alone, so that a failed verification run cannot surface in a live API
response or corrupt the corpus.

#### Scenario: A canary run is executed

- **WHEN** the pipeline's queue, heartbeat, and worker behavior are verified
- **THEN** all writes SHALL target buckets provisioned solely for testing, not
  the six adopted production buckets

#### Scenario: A test write could otherwise reach production

- **WHEN** a bucket is both a batch-pipeline write target and a fallback the
  serving plane reads from
- **THEN** its test counterpart SHALL be a distinct bucket, not a prefix or
  object-naming convention within the same bucket

### Requirement: Workload deployment stays declarative and reproducible

The batch cluster's workloads SHALL be deployed from version-controlled
manifests via an automated, auditable path, and SHALL NOT be applied by hand
against a live cluster.

#### Scenario: A workload is deployed or updated

- **WHEN** a controller, worker, or support workload is deployed or updated
- **THEN** the manifest applied SHALL come from this repository's version
  control, applied by CI rather than manually

#### Scenario: CI authenticates to the cluster

- **WHEN** CI deploys to the batch cluster
- **THEN** it SHALL use short-lived federated credentials whose trust is
  constrained to this repository and a protected environment, and SHALL NOT
  use long-lived stored keys
