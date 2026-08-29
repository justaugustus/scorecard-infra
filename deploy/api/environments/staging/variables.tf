variable "region" {
  description = <<-EOT
    Observed, not chosen: all seven corpus buckets are in us-east-1, and compute
    has to share their region or lose the S3 gateway endpoint and pay NAT egress
    on every object read (A5).
  EOT
  type        = string
  default     = "us-east-1"
}

variable "availability_zones" {
  description = "Override the AZs. Null picks the first two available."
  type        = list(string)
  default     = null
}

variable "image" {
  description = <<-EOT
    Container image pinned by digest. Staging tracks the `main` tag, resolved to
    a digest before it reaches here -- deploying the tag itself would mean a
    rollback lands on whatever `main` points at that day.
  EOT
  type        = string
}

variable "origin_hostname" {
  description = <<-EOT
    Hostname Fastly's staging backend will point at, e.g.
    origin-staging.scorecard.dev.

    No default, deliberately. This is NOT api-staging.scorecard.dev -- that is
    the CDN hostname in front of Fastly. This is the origin behind it, a new
    record created in Netlify DNS, and guessing it would produce a certificate
    for a name nobody intended to publish.
  EOT
  type        = string
}

variable "api_base_url" {
  description = <<-EOT
    Public base URL the API uses to build purge requests. Production is
    https://api.scorecard.dev, captured from the running service. Confirm the
    staging equivalent rather than assuming symmetry: a wrong value purges the
    wrong hostname, and a failed purge is indistinguishable from a cache that
    has not expired.
  EOT
  type        = string
}

variable "results_bucket" {
  description = "Primary results bucket. Shared with production, read-only here."
  type        = string
  default     = "ossf-scorecard-results"
}

variable "cron_results_bucket" {
  description = "Fallback results bucket. Read-only in both environments."
  type        = string
  default     = "ossf-scorecard-cron-results"
}

variable "github_repository" {
  description = "The one repository allowed to deploy, as owner/name."
  type        = string
  default     = "ossf/scorecard-infra"
}
