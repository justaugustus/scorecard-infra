variable "name" {
  description = "Name for the deploy role."
  type        = string
}

variable "github_repository" {
  description = <<-EOT
    The one repository allowed to assume this role, as `owner/name`.

    Scoped to a repository rather than an organisation on purpose: an org-wide
    trust means any repository anyone creates in the org can deploy this
    service.
  EOT
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$", var.github_repository))
    error_message = "Must be owner/name -- a bare name or a wildcard would widen the trust."
  }
}

variable "github_environment" {
  description = <<-EOT
    The GitHub Actions environment the trust is bound to, e.g. `production`.

    This is the load-bearing half of the trust policy. Binding to a branch
    instead would let anyone who can push a branch -- or open a pull request
    that runs a workflow -- assume the role. An environment can carry required
    reviewers and branch restrictions, which is what actually gates the deploy.
  EOT
  type        = string
}

variable "create_oidc_provider" {
  description = <<-EOT
    Whether to create the GitHub OIDC provider. AWS permits one per issuer per
    account, so the second environment to be applied must set this false and
    pass the existing ARN. The account had none at capture time.
  EOT
  type        = bool
  default     = true
}

variable "existing_oidc_provider_arn" {
  description = "ARN of an already-created GitHub OIDC provider, when create_oidc_provider is false."
  type        = string
  default     = null
}

variable "ecs_cluster_arn" {
  description = "Cluster the deploy role may act on."
  type        = string
}

variable "ecs_service_arn" {
  description = "The one service the deploy role may update."
  type        = string
}

variable "task_definition_family" {
  description = "Task definition family the deploy role may register revisions of."
  type        = string
}

variable "passable_role_arns" {
  description = <<-EOT
    The task and execution roles, and nothing else.

    Registering a task definition means naming the roles the task will run as,
    so this role needs iam:PassRole -- and an unscoped PassRole is a privilege
    escalation, not a convenience: it would let CI run a task as any role in the
    account, including an administrative one.
  EOT
  type        = list(string)
}

variable "tags" {
  description = "Tags applied to every resource, so anything untagged reads as drift."
  type        = map(string)
  default     = {}
}
