# Staging: the AWS-backed origin the cutover rehearses against.
#
# This is where the migration is actually proven. Fastly's STAGING backend
# points here first and conformance runs against it; flipping the production
# backend afterwards is the identical operation, which is what makes this a
# rehearsal rather than an approximation.
#
# Staging reads the SAME corpus as production and cannot write to it. See
# enable_publish_writes in modules/service -- conformance against an empty
# bucket would prove nothing, but a staging build must never be able to publish
# over production results.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # Partial configuration: the bucket is chosen at bootstrap, so it is supplied
  # at init rather than committed here.
  #
  #   tofu init -backend-config=bucket=<state bucket>
  #
  backend "s3" {
    key          = "api/staging/terraform.tfstate"
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
  name = "scorecard-api-staging"

  tags = {
    Project     = "scorecard"
    Component   = "api"
    Environment = "staging"
    ManagedBy   = "opentofu"
    Source      = "ossf/scorecard-infra//deploy/api"
  }

  azs = var.availability_zones != null ? var.availability_zones : slice(data.aws_availability_zones.available.names, 0, 2)
}

data "aws_availability_zones" "available" {
  state = "available"
}

# Adopted, never managed (A13). These hold the corpus and DataSync writes to
# them; declaring them as resources would put that data one `tofu destroy` or
# one deleted block away from deletion.
#
# The data sources earn their place beyond documentation: they fail the PLAN if
# a bucket name is wrong or unreachable, rather than letting the service deploy
# and 404 every request at runtime.
data "aws_s3_bucket" "results" {
  bucket = var.results_bucket
}

data "aws_s3_bucket" "cron_results" {
  bucket = var.cron_results_bucket
}

module "network" {
  source = "../../modules/network"

  name               = local.name
  availability_zones = local.azs
  tags               = local.tags
}

module "secrets" {
  source = "../../modules/secrets"

  name_prefix = "scorecard/staging"
  tags        = local.tags
}

module "edge" {
  source = "../../modules/edge"

  name              = local.name
  vpc_id            = module.network.vpc_id
  public_subnet_ids = module.network.public_subnet_ids
  origin_hostname   = var.origin_hostname
  tags              = local.tags
}

module "service" {
  source = "../../modules/service"

  name  = local.name
  image = var.image

  results_bucket      = data.aws_s3_bucket.results.bucket
  cron_results_bucket = data.aws_s3_bucket.cron_results.bucket
  api_base_url        = var.api_base_url

  # Read-only against the shared corpus. The whole point of the flag.
  enable_publish_writes = false

  fastly_secret_arn        = module.secrets.secret_arns["fastly"]
  secrets_read_policy_json = module.secrets.read_policy_json

  private_subnet_ids = module.network.private_subnet_ids
  security_group_id  = module.edge.tasks_security_group_id
  target_group_arn   = module.edge.target_group_arn

  tags = local.tags
}

module "ci_oidc" {
  source = "../../modules/ci-oidc"

  name               = "${local.name}-deploy"
  github_repository  = var.github_repository
  github_environment = "staging"

  # Staging is applied first, so it creates the shared provider. Production
  # consumes it -- AWS allows one per issuer per account.
  create_oidc_provider = true

  ecs_cluster_arn        = module.service.cluster_arn
  ecs_service_arn        = module.service.service_arn
  task_definition_family = module.service.task_definition_family

  passable_role_arns = [
    module.service.task_role_arn,
    module.service.execution_role_arn,
  ]

  tags = local.tags
}
