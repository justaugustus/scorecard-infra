# aws-serving Specification

## Purpose
Defines how the results API is deployed and served from AWS: what binary runs,
how compute and storage are placed relative to each other, how the serving
plane stays isolated from the batch plane, what the CDN's origin must look
like, how workloads are deployed and identified, and how the AWS-backed path
is proven equivalent to production before it takes traffic.
## Requirements
### Requirement: The deployed serving binary is the imported API

The AWS deployment SHALL run the imported results API (`api/`), which is the
server that ships. It SHALL NOT deploy the provider-agnostic hybrid server, which
is off the deployment path.

#### Scenario: Compute is provisioned for the serving path

- **WHEN** the serving path is deployed to AWS
- **THEN** the artifact deployed SHALL be the imported API's image, and SHALL NOT
  be the hybrid server's

### Requirement: Compute and storage are co-located in one region

The serving compute and the result buckets SHALL be in the same region, and that
region SHALL be determined from where the buckets already are rather than
chosen. A cross-region deployment forfeits the S3 gateway endpoint and pays
per-gigabyte egress on every object read, on a service whose workload is object
reads.

#### Scenario: The region is configured

- **WHEN** the region is set
- **THEN** it SHALL be a required input with no default, populated from a capture
  of the account

#### Scenario: Object access does not traverse NAT

- **WHEN** a workload reads from a result bucket
- **THEN** the path SHALL be an S3 gateway endpoint rather than NAT egress

### Requirement: The serving and batch planes are deployed independently

The internet-facing serving plane and the batch scanning plane SHALL be separate
deployments with separate identities, and neither SHALL depend on compute the
other operates. The serving plane accepts traffic from the internet and the
batch plane does not, so they warrant different blast radii; the batch plane is
also the more complex system, and its complexity should not sit in the path of a
public API.

#### Scenario: One plane is provisioned

- **WHEN** either plane is provisioned, changed, or destroyed
- **THEN** the other SHALL continue serving or scanning unaffected

#### Scenario: A workload assumes an identity

- **WHEN** a workload obtains cloud credentials
- **THEN** the identity SHALL belong to that workload alone, and SHALL NOT be
  assumable by the other plane

#### Scenario: Batch load cannot starve serving

- **WHEN** the batch workload scales out
- **THEN** it SHALL NOT consume capacity the serving workload requires

#### Scenario: Permissions are least-privilege

- **WHEN** an identity is granted access
- **THEN** it SHALL reach only the buckets, queues, and secrets that workload
  requires, and SHALL NOT reproduce the breadth of a shared default account

#### Scenario: Denial is verified, not only permission

- **WHEN** an identity's permissions are tested
- **THEN** the test SHALL confirm that resources outside its grant are refused,
  not only that resources inside it are reachable

### Requirement: The CDN origin is a verifiable hostname under independent lifecycle

The origin the CDN points at SHALL be a hostname terminating TLS with a
certificate the CDN can verify, and its lifecycle SHALL be independent of any
workload object. A bare IP would require overriding SNI and the host header plus
either a certificate issued for an IP or disabled origin verification — a
security-posture change inside a migration whose acceptance claim is that
behavior did not change.

#### Scenario: The CDN connects to the origin

- **WHEN** the CDN connects
- **THEN** the origin SHALL present a valid certificate for its hostname, with no
  SNI override and no disabled verification

#### Scenario: A workload object is deleted

- **WHEN** a workload object referencing the origin is deleted or recreated
- **THEN** the origin's hostname SHALL be unchanged, because the cutover and its
  rollback both turn on that hostname

### Requirement: Deployments name an immutable artifact

Workloads SHALL be deployed by immutable digest rather than by mutable tag, so
that what is running can be named exactly and a rollback targets a known build.

#### Scenario: A workload is deployed

- **WHEN** a workload is deployed
- **THEN** its image SHALL be referenced by digest

#### Scenario: CI assumes a deployment identity

- **WHEN** CI authenticates to the cloud account
- **THEN** it SHALL use short-lived federated credentials whose trust is
  constrained to this repository and a protected environment, and SHALL NOT use
  long-lived stored keys

### Requirement: Equivalence is measured against production before traffic moves

Before any traffic is moved, the AWS-backed path SHALL be compared against the
running production path, and the comparison SHALL be made between origins rather
than between CDN hostnames.

#### Scenario: Parity is checked

- **WHEN** the AWS path is compared to production
- **THEN** the comparison SHALL cover status codes, response bodies, and cache
  directives

#### Scenario: The comparison is not distorted by caching

- **WHEN** two paths are compared
- **THEN** the comparison SHALL be made origin-to-origin, because published
  hostnames cache separately and a CDN-level comparison measures cache vintage
  rather than behavior

#### Scenario: A request-path component is removed

- **WHEN** a component is removed from the request path
- **THEN** its contribution SHALL be measured before removal, and each difference
  SHALL be either reconciled or accepted deliberately

