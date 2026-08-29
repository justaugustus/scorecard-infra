variable "name_prefix" {
  description = "Prefix for secret names, e.g. scorecard/staging."
  type        = string
}

variable "secrets" {
  description = <<-EOT
    Secrets to create, keyed by short name. Values are NOT set here and never
    should be.

    The default is the serving plane's complete set: one. The API reads
    FASTLY_PURGE_TOKEN and nothing else credential-shaped. The github and gitlab
    secrets are the batch plane's.
  EOT
  type = map(object({
    description = string
  }))
  default = {
    fastly = {
      description = "Fastly purge token. The API purges a result's URL after a successful publish."
    }
  }
}

variable "recovery_window_days" {
  description = <<-EOT
    Grace period before a deleted secret is unrecoverable. The value is not in
    OpenTofu state, so this window is the only way back from an accidental
    delete. 0 would mean immediate, irreversible deletion.
  EOT
  type        = number
  default     = 30

  validation {
    condition     = var.recovery_window_days >= 7
    error_message = "Keep at least 7 days: the value is not in state, so there is nothing else to restore from."
  }
}

variable "tags" {
  description = "Tags applied to every resource, so anything untagged reads as drift."
  type        = map(string)
  default     = {}
}
