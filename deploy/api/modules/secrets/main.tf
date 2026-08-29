# Secrets Manager containers for the serving plane.
#
# This module creates the secret RESOURCES and nothing else. It deliberately
# does not create aws_secretsmanager_secret_version, because that is the
# resource that carries the value -- and a value passed to OpenTofu is a value
# written to state, which would turn the state bucket into a credential store
# with weaker controls than the service built for the purpose (A11).
#
# There is consequently no `ignore_changes` here and nothing to drift: OpenTofu
# has no opinion about the contents. Load them out of band:
#
#   aws secretsmanager put-secret-value \
#     --secret-id scorecard/staging/fastly \
#     --secret-string file://token.json
#
# Scope note: the API reads exactly ONE secret, FASTLY_PURGE_TOKEN -- verified
# against api/app/server/post_results.go:639, which is the only getenv for a
# credential in the whole tree. The github and gitlab secrets belong to the
# batch plane, which reads them; creating them here would be creating them in
# the wrong place, and creating them in both is worse than either.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

resource "aws_secretsmanager_secret" "this" {
  for_each = var.secrets

  name        = "${var.name_prefix}/${each.key}"
  description = each.value.description

  # Deleting a secret is almost always a mistake discovered shortly afterwards.
  # The recovery window is the only thing standing between that mistake and an
  # unrecoverable one, since the value is not in state to restore from.
  recovery_window_in_days = var.recovery_window_days

  tags = merge(var.tags, { Name = "${var.name_prefix}/${each.key}" })
}

# Read access, for attaching to whichever role needs it. Scoped to exactly these
# secrets: a wildcard here would grant the task every secret in the account,
# including the batch plane's and DataSync's.
data "aws_iam_policy_document" "read" {
  statement {
    sid    = "ReadOwnSecrets"
    effect = "Allow"

    actions = [
      "secretsmanager:GetSecretValue",
      "secretsmanager:DescribeSecret",
    ]

    resources = [for s in aws_secretsmanager_secret.this : s.arn]
  }
}
