# AWS serving environment

OpenTofu for the results API — the `api/` tree, which is the server that ships.

This deploys **only the serving plane**. The batch pipeline is a separate
deployment under `deploy/cron/` and shares nothing here but the account. See
`openspec/changes/provision-aws/` for the proposal, the decision record
(**A1**–**A15**), and the task list.

## Requirements

- **OpenTofu >= 1.10.** Not optional: state locking uses `use_lockfile`, native
  S3 conditional writes, which does not exist before 1.10 — v1.9 documents only
  `dynamodb_table` and disables locking silently without it. Each root module
  declares the constraint; `../.opentofu-version` pins the exact version for
  both planes, and version managers of the `tenv`/`tofuenv` family find it by
  searching upward from wherever you are standing.
- AWS credentials for the target account.
- **Run off any TLS-intercepting network.** A VPN that re-signs certificates
  makes the AWS SDK fail with `SSL validation failed ... self-signed certificate
  in certificate chain`, which reads like a permissions error and is not one.

## Layout

```text
bootstrap/       one-time; creates the state bucket (see below)
modules/         building blocks, internal to this deployment
environments/
  staging/       root module, own state key
  production/    root module, own state key
```

Each root module is independent — `bootstrap/`, `environments/staging/`, and
`environments/production/` each carry their own `terraform` block and provider
configuration. Terraform and OpenTofu do not inherit those from a parent
directory, so there is no tree-wide `versions.tf` and the duplication is
required rather than sloppy.

Environments are **separate root modules with separate state keys, not
workspaces**. Workspaces share one backend configuration, which makes it
possible to apply to production while believing you are in staging. Here the
target is the directory you are standing in.

## One-time bootstrap

The bucket that holds state has to exist before there is a backend to record
its creation. That circle is broken once, by hand:

```sh
cd bootstrap

# 1. Apply with LOCAL state. The backend block is commented out on purpose.
tofu init
tofu apply -var region=<region> -var state_bucket_name=<bucket>

# 2. Uncomment the backend block in main.tf, filling in the bucket and region
#    from the outputs above.

# 3. Move this root module's own state into the bucket it just created.
tofu init -migrate-state

# 4. The local state file is now a copy, not the source of truth.
rm -f terraform.tfstate terraform.tfstate.backup
```

**After step 4, a `terraform.tfstate` in any directory here is a mistake.** It
means someone ran a root module whose backend block was missing or commented
out, and their changes are recorded only on their laptop.

The region is deliberately a required variable with no default. It is an
observation — wherever the result buckets already are — not a choice; a service
in a different region from its buckets loses the S3 gateway endpoint and pays
NAT egress on every object read (**A5**).

## Normal use

```sh
cd environments/staging
tofu init
tofu plan
tofu apply
```

## What this does not manage

- **The corpus buckets.** They already exist and hold data, so they are
  referenced as `data` sources and never declared as resources (**A13**).
  Declaring them would put the corpus one `tofu destroy`, one `-target`
  mistake, or one deleted block away from deletion — and OpenTofu cannot tell
  "this should not exist" from "someone removed the block that declared it."
  Their versioning and lifecycle settings are out of scope here for the same
  reason.
- **Secret values.** The secret *resources* and their access policies are
  managed; the values are loaded out-of-band and `ignore_changes` covers them. A
  value passed to OpenTofu is a value written to state, which would make the
  state bucket a credential store with weaker controls than the service built
  for it (**A11**).
- **DNS.** Both zones are delegated to Netlify DNS, confirmed by the account
  holding zero Route 53 zones. Two record sets have to be created there by hand,
  and both are emitted as outputs:
  1. the **ACM validation records** — `aws_acm_certificate_validation` blocks
     until they exist, so this shows up as a stalled apply rather than a silent
     prerequisite;
  2. the **origin hostname**, pointing at the ALB.

## The origin

The CDN is pinned to the ALB hostname, the production cutover is a single
backend field in Fastly, and rollback is restoring that field. So the load
balancer and its certificate are first-class resources here with a lifecycle
independent of any workload — nothing should be able to change or destroy the
origin as a side effect of a deployment (**A9**, **A10**).
