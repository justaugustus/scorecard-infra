output "state_bucket" {
  description = <<-EOT
    Name of the state bucket. Put this in the backend block of this directory
    and of each environment root module, alongside `use_lockfile = true`.
  EOT
  value       = module.state_backend.bucket
}

output "state_bucket_region" {
  description = "Region for the backend block."
  value       = module.state_backend.region
}
