# The API on ECS Fargate.
#
# No nodes, no OS to patch, no control plane to upgrade -- the surface you do
# not operate is the one that cannot be left unpatched. Fargate bills per
# running task-second rather than per request, so this does not reintroduce the
# per-request metering the migration exists to escape (A7).
#
# Two roles, deliberately distinct:
#   * the EXECUTION role belongs to the ECS agent -- it pulls the image, writes
#     logs, and resolves secrets into the container environment before the
#     process starts;
#   * the TASK role belongs to the application -- it is what the API's own S3
#     calls authenticate as.
# Conflating them would hand the application the ability to read every secret
# the agent can resolve.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

data "aws_region" "current" {}

locals {
  results_arn      = "arn:aws:s3:::${var.results_bucket}"
  cron_results_arn = "arn:aws:s3:::${var.cron_results_bucket}"
}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.name}"
  retention_in_days = var.log_retention_days

  tags = var.tags
}

# --- Execution role (the ECS agent) -----------------------------------------

data "aws_iam_policy_document" "assume_ecs_tasks" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${var.name}-execution"
  assume_role_policy = data.aws_iam_policy_document.assume_ecs_tasks.json
  tags               = var.tags
}

# Scoped by hand rather than attaching AmazonECSTaskExecutionRolePolicy, which
# carries ECR pull permissions this deployment has no use for -- images come
# from ghcr.io, and no registry is created in this account.
data "aws_iam_policy_document" "execution" {
  statement {
    sid    = "WriteOwnLogs"
    effect = "Allow"

    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]

    resources = ["${aws_cloudwatch_log_group.this.arn}:*"]
  }
}

resource "aws_iam_role_policy" "execution_logs" {
  name   = "logs"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution.json
}

# The agent resolves the secret into the environment before the process starts,
# so this belongs to the execution role, not the task role.
resource "aws_iam_role_policy" "execution_secrets" {
  name   = "secrets"
  role   = aws_iam_role.execution.id
  policy = var.secrets_read_policy_json
}

# --- Task role (the application) --------------------------------------------

resource "aws_iam_role" "task" {
  name               = "${var.name}-task"
  assume_role_policy = data.aws_iam_policy_document.assume_ecs_tasks.json
  tags               = var.tags
}

# Asymmetric on purpose, and verified against the code rather than assumed:
# the publish path writes to the PRIMARY bucket only (post_results.go:167 ->
# :279), while the read path tries the primary and then falls back to the cron
# bucket (get_results.go:82, :94). So the fallback is read-only. A uniform grant
# would either break publishing or hand out a write it does not need.
data "aws_iam_policy_document" "task" {
  statement {
    sid       = "ReadPrimaryResults"
    effect    = "Allow"
    actions   = ["s3:GetObject"]
    resources = ["${local.results_arn}/*"]
  }

  # Split from the read above rather than folded into it, because staging and
  # production share a corpus and must not share this. Conformance needs real
  # data to be meaningful, so staging reads the production buckets -- but the
  # publish path writes into whichever bucket it reads as primary, so a staging
  # task with PutObject could overwrite production results with output from an
  # unproven build. Reads are shared; writes are production-only.
  dynamic "statement" {
    for_each = var.enable_publish_writes ? [1] : []

    content {
      sid       = "WritePrimaryResults"
      effect    = "Allow"
      actions   = ["s3:PutObject"]
      resources = ["${local.results_arn}/*"]
    }
  }

  statement {
    sid    = "ReadFallbackResults"
    effect = "Allow"

    actions   = ["s3:GetObject"]
    resources = ["${local.cron_results_arn}/*"]
  }

  # Not incidental. Without s3:ListBucket, S3 answers a GET for a missing key
  # with 403 rather than 404 -- so a repository that has simply never been
  # scanned would surface as an error instead of "not found", and the
  # conformance harness checks exactly that case.
  statement {
    sid    = "DistinguishMissingFromForbidden"
    effect = "Allow"

    actions = ["s3:ListBucket"]

    resources = [
      local.results_arn,
      local.cron_results_arn,
    ]
  }
}

resource "aws_iam_role_policy" "task" {
  name   = "results-buckets"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task.json
}

# --- Task definition and service --------------------------------------------

resource "aws_ecs_cluster" "this" {
  name = var.name

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = var.tags
}

resource "aws_ecs_task_definition" "this" {
  family                   = var.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([
    {
      name      = "api"
      image     = var.image
      essential = true

      portMappings = [
        {
          containerPort = var.container_port
          protocol      = "tcp"
        },
      ]

      # The bucket URLs carry ?region= because gocloud's s3blob resolves the
      # region from the URL rather than from the task's environment.
      environment = [
        {
          name  = "API_BASE_URL"
          value = var.api_base_url
        },
        {
          name  = "SCORECARD_RESULTS_BUCKET_URL"
          value = "s3://${var.results_bucket}?region=${data.aws_region.current.region}"
        },
        {
          name  = "SCORECARD_CRON_RESULTS_BUCKET_URL"
          value = "s3://${var.cron_results_bucket}?region=${data.aws_region.current.region}"
        },
      ]

      # The only credential the API reads, verified against
      # api/app/server/post_results.go:639.
      secrets = [
        {
          name      = "FASTLY_PURGE_TOKEN"
          valueFrom = "${var.fastly_secret_arn}:${var.fastly_secret_json_key}::"
        },
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.this.name
          "awslogs-region"        = data.aws_region.current.region
          "awslogs-stream-prefix" = "api"
        }
      }
    },
  ])

  tags = var.tags
}

resource "aws_ecs_service" "this" {
  name            = var.name
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.security_group_id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = var.target_group_arn
    container_name   = "api"
    container_port   = var.container_port
  }

  # Keep at least the current capacity serving through a deployment. With two
  # tasks this replaces them one at a time rather than dropping to half.
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  # Roll back automatically on a deployment that never goes healthy, rather than
  # leaving a half-replaced service for someone to notice.
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  # The container opens its listener quickly; this is slack for image pull and
  # start, not for warm-up.
  health_check_grace_period_seconds = 60

  propagate_tags = "SERVICE"
  tags           = var.tags
}
