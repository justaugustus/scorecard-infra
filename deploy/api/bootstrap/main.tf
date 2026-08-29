# One-time bootstrap: creates the S3 bucket that holds every other root
# module's state.
#
# This is the circular part of the design. The bucket that stores state has to
# exist before there is a backend to record its creation, so this root module is
# applied once with LOCAL state and then migrates its own state into the bucket
# it just made. Full procedure in deploy/api/README.md.
#
# After that migration, a terraform.tfstate file in this directory is a mistake,
# not a leftover.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # Uncomment ONLY after the first apply has created the bucket, then run
  # `tofu init -migrate-state`. Leaving this uncommented on a fresh account
  # fails at init, because the bucket named here does not exist yet.
  #
  # backend "s3" {
  #   bucket       = "<state_bucket_name>"
  #   key          = "bootstrap/terraform.tfstate"
  #   region       = "us-east-1"
  #   encrypt      = true
  #   use_lockfile = true
  # }
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
    Component = "state-backend"
    ManagedBy = "opentofu"
    Source    = "ossf/scorecard-infra//deploy/api"
  }
}

module "state_backend" {
  source = "../modules/state-backend"

  bucket_name = var.state_bucket_name
  tags        = local.tags
}
