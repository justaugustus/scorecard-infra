variable "name" {
  description = "Name prefix for the cluster, service, and roles."
  type        = string
}

variable "image" {
  description = <<-EOT
    Container image, PINNED BY DIGEST:
      ghcr.io/<owner>/scorecard-api@sha256:...

    Not a tag. A tag is a mutable pointer, so a rollback to a tag is a rollback
    to whatever that tag means today rather than to a known build. Staging
    tracks `main`, production tracks `api/v*`, and both resolve to a digest
    before they reach here.
  EOT
  type        = string

  validation {
    condition     = can(regex("@sha256:[0-9a-f]{64}$", var.image))
    error_message = "Image must be pinned by digest (…@sha256:<64 hex>), not by tag."
  }
}

variable "results_bucket" {
  description = <<-EOT
    Primary results bucket, e.g. ossf-scorecard-results. Both READ and WRITTEN:
    the read path tries it first (get_results.go) and the publish path writes
    results into it (post_results.go).
  EOT
  type        = string
}

variable "cron_results_bucket" {
  description = <<-EOT
    Fallback bucket the read path tries when a repository is absent from the
    primary, e.g. ossf-scorecard-cron-results. READ ONLY -- nothing in the API
    writes here, so nothing should be permitted to.
  EOT
  type        = string
}

variable "api_base_url" {
  description = <<-EOT
    Public base URL, e.g. https://api.scorecard.dev. Used to build the URL the
    API purges from Fastly after a publish -- so a wrong value here means purges
    silently hit the wrong hostname, and a failed purge is indistinguishable
    from a cache that has not expired.
  EOT
  type        = string
}

variable "fastly_secret_arn" {
  description = "ARN of the Fastly secret. Expected to be JSON with a purge_token key."
  type        = string
}

variable "fastly_secret_json_key" {
  description = "Key within the Fastly secret JSON holding the purge token."
  type        = string
  default     = "purge_token"
}

variable "secrets_read_policy_json" {
  description = "IAM policy from the secrets module, scoped to this plane's secrets."
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnets. Tasks get no public IP."
  type        = list(string)
}

variable "security_group_id" {
  description = "Task security group, from the edge module. Ingress from the load balancer only."
  type        = string
}

variable "target_group_arn" {
  description = "Target group to register tasks into, from the edge module."
  type        = string
}

variable "container_port" {
  description = "Port the container listens on. api/Dockerfile: --port=8080."
  type        = number
  default     = 8080
}

variable "cpu" {
  description = <<-EOT
    Task CPU units. 512 = 0.5 vCPU.

    Cloud Run runs this service at 1 vCPU / 512Mi with concurrency 120. That
    bounds the per-task request but says nothing about the floor, because Cloud
    Run scales to zero and Fargate does not. Provisional (A8) -- the measurement
    that should replace it is origin request rate, which is cache misses per
    second rather than public traffic, and conformance has to run first.
  EOT
  type        = number
  default     = 512
}

variable "memory" {
  description = "Task memory (MiB). Fargate constrains valid pairings with cpu."
  type        = number
  default     = 1024
}

variable "desired_count" {
  description = <<-EOT
    Task count. Two is an availability floor across two AZs, not a capacity
    measurement -- see cpu.
  EOT
  type        = number
  default     = 2
}

variable "log_retention_days" {
  description = <<-EOT
    CloudWatch log retention. Finite on purpose: the default is "never expire",
    which is a slow cost leak rather than a durability feature.
  EOT
  type        = number
  default     = 30
}

variable "tags" {
  description = "Tags applied to every resource, so anything untagged reads as drift."
  type        = map(string)
  default     = {}
}
