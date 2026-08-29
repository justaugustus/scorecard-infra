# The bucket holding OpenTofu state for every root module in this tree.
#
# Locking is native S3 conditional writes (`use_lockfile = true` in the backend
# block of each root module), not DynamoDB -- one less resource to create, own,
# pay for, and forget to grant access to. That requires OpenTofu >= 1.10; see
# A3 and the required_version constraint in each root module.
#
# This module is applied ONCE, from deploy/api/bootstrap/, with local state,
# after which its own state is migrated into the bucket it just created. See
# deploy/api/README.md.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

resource "aws_s3_bucket" "state" {
  bucket = var.bucket_name

  # This bucket outlives every stack whose state it holds, and losing it is the
  # one failure the rest of this design cannot recover from -- the state
  # describing the infrastructure would go with it.
  lifecycle {
    prevent_destroy = true
  }

  tags = var.tags
}

# Upstream advises this explicitly, and it is the recovery path for a state file
# that gets truncated or corrupted by an interrupted apply.
resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }

    # Cuts KMS/S3 encryption request costs on a bucket written on every apply.
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket = aws_s3_bucket.state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# State contains resource attributes and can contain sensitive values. Public
# access is already blocked above; this closes the other half by refusing
# plaintext transport outright rather than trusting every future client to opt
# in to TLS.
data "aws_iam_policy_document" "state" {
  statement {
    sid    = "DenyInsecureTransport"
    effect = "Deny"

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    actions = ["s3:*"]

    resources = [
      aws_s3_bucket.state.arn,
      "${aws_s3_bucket.state.arn}/*",
    ]

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "state" {
  bucket = aws_s3_bucket.state.id
  policy = data.aws_iam_policy_document.state.json

  # A policy that denies everything to everyone is what a misordered apply looks
  # like here, so make the public-access block land first.
  depends_on = [aws_s3_bucket_public_access_block.state]
}

# Every apply supersedes a state version, and native locking leaves a lock
# object behind if a run is killed mid-apply. Neither should accumulate
# indefinitely.
resource "aws_s3_bucket_lifecycle_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    id     = "expire-noncurrent-state"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = var.noncurrent_version_retention_days
    }
  }

  rule {
    id     = "abort-incomplete-uploads"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  depends_on = [aws_s3_bucket_versioning.state]
}
