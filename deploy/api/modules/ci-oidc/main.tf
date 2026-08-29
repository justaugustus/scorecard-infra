# GitHub Actions deploys via OIDC: short-lived credentials from a trust
# relationship, rather than an access key living in a repository secret. There
# is no key to rotate, leak, or forget to revoke.
#
# Scope: this role deploys the APPLICATION -- it registers a task definition
# pointing at a new image digest and updates the service. It deliberately cannot
# change infrastructure. `tofu apply` stays a human action, because a role that
# can apply arbitrary OpenTofu is a role that can rewrite its own trust policy.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

resource "aws_iam_openid_connect_provider" "github" {
  count = var.create_oidc_provider ? 1 : 0

  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]

  # AWS verifies this issuer against its own trust store and ignores the
  # thumbprint, but the argument is still part of the resource. Kept current so
  # nothing depends on the value being meaningful.
  thumbprint_list = [
    "6938fd4d98bab03faadb97b34396831e3780aea1",
    "1c58a3a8518e8759bf075b76b750d4f2df264fcd",
  ]

  tags = var.tags
}

locals {
  oidc_provider_arn = var.create_oidc_provider ? aws_iam_openid_connect_provider.github[0].arn : var.existing_oidc_provider_arn
}

data "aws_iam_policy_document" "assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    # Both conditions matter. Without `aud`, a token minted for another
    # audience could be replayed here.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    # StringEquals, not StringLike: no wildcard anywhere in the subject. This
    # names one repository and one environment, so a fork, another repository in
    # the org, or a workflow running outside the environment all fail to assume.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_repository}:environment:${var.github_environment}"]
    }
  }
}

resource "aws_iam_role" "deploy" {
  name               = var.name
  description        = "GitHub Actions deploy role. Application deploys only, not infrastructure."
  assume_role_policy = data.aws_iam_policy_document.assume.json

  # A deploy should take seconds. A long session is a long window in which a
  # leaked token is still usable.
  max_session_duration = 3600

  tags = var.tags
}

data "aws_iam_policy_document" "deploy" {
  # RegisterTaskDefinition takes no resource ARN -- AWS does not support
  # resource-level permissions for it, so this is the one broad grant here. The
  # narrowing that matters is PassRole below: a task definition is only
  # dangerous if it can name a role worth stealing.
  statement {
    sid       = "RegisterTaskDefinitions"
    effect    = "Allow"
    actions   = ["ecs:RegisterTaskDefinition", "ecs:DescribeTaskDefinition"]
    resources = ["*"]
  }

  statement {
    sid    = "UpdateOwnService"
    effect = "Allow"

    actions = [
      "ecs:UpdateService",
      "ecs:DescribeServices",
    ]

    resources = [var.ecs_service_arn]
  }

  statement {
    sid       = "ObserveRollout"
    effect    = "Allow"
    actions   = ["ecs:DescribeTasks", "ecs:ListTasks"]
    resources = ["*"]

    condition {
      test     = "ArnEquals"
      variable = "ecs:cluster"
      values   = [var.ecs_cluster_arn]
    }
  }

  # The escalation guard. Scoped to exactly the task and execution roles, and
  # further conditioned on the service they may be passed to -- so even those
  # two ARNs cannot be handed to something other than ECS.
  statement {
    sid       = "PassTaskRolesToEcsOnly"
    effect    = "Allow"
    actions   = ["iam:PassRole"]
    resources = var.passable_role_arns

    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "deploy" {
  name   = "deploy"
  role   = aws_iam_role.deploy.id
  policy = data.aws_iam_policy_document.deploy.json
}
