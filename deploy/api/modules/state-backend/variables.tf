variable "bucket_name" {
  description = "Name of the S3 bucket holding OpenTofu state. Globally unique."
  type        = string
}

variable "noncurrent_version_retention_days" {
  description = <<-EOT
    How long superseded state versions are kept. The recovery case this serves
    is a bad apply noticed days or weeks later, so it is deliberately longer
    than a rollback window; it is finite because state accumulates a version
    per apply forever otherwise.
  EOT
  type        = number
  default     = 90

  validation {
    condition     = var.noncurrent_version_retention_days >= 30
    error_message = "Keep at least 30 days: state recovery is the only reason this bucket is versioned."
  }
}

variable "tags" {
  description = "Tags applied to every resource, so anything untagged reads as drift."
  type        = map(string)
  default     = {}
}
