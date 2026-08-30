# Secrets Manager containers for the batch (scanning) plane.
#
# The serving plane's secrets module (deploy/api/modules/secrets) deliberately
# excludes these: it reads exactly one credential (FASTLY_PURGE_TOKEN), and its
# own comment says creating github/gitlab there would put the batch plane's
# secrets in the serving plane's module, which is the wrong place per
# aws-serving's plane-independence requirement. This root gives them their own
# home instead of leaving them undeclared, which is what migrate-api 6.0a
# flagged: the destination is decided (AWS Secrets Manager, this account), but
# no naming scheme or IAM policy exists yet.
#
# Like its sibling, this declares containers only. No aws_secretsmanager_secret_version
# here -- load values out of band, e.g. with scripts/cutover/load-secrets.sh.
#
# This does not stand up any batch/cron compute. That migration is its own,
# separate, deferred thread; this root only reserves where its credentials
# live, so they have a destination before GCP access ends.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # Partial configuration; bucket supplied at init:
  #   tofu init -backend-config=bucket=<state bucket>
  backend "s3" {
    key          = "cron/secrets/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = local.tags
  }
}

locals {
  tags = {
    Project   = "scorecard"
    Component = "batch"
    ManagedBy = "opentofu"
    Source    = "ossf/scorecard-infra//deploy/cron/secrets"
  }
}

module "secrets" {
  source = "../../api/modules/secrets"

  name_prefix = "scorecard/batch"
  tags        = local.tags

  # One secret per Kubernetes Secret found in the openssf/default GKE
  # namespace (migrate-api 6.0a), collapsed to what the batch plane actually
  # needs. criticality-score's github/enumerate-github-auth/github-staging are
  # a different hosted service's credentials and are explicitly out of scope
  # here -- flagged in 6.0a as needing their own owner, not folded in here.
  secrets = {
    github = {
      description = "GitHub App credentials the scanning side authenticates as (app_id, app_key, installation_id, token)."
    }
    gitlab = {
      description = "GitLab auth token. Scorecard scans GitLab repositories with this."
    }
  }
}
