output "secret_arns" {
  description = "Secret ARNs keyed by short name, for whichever batch compute eventually reads them."
  value       = module.secrets.secret_arns
}

output "read_policy_json" {
  description = "IAM policy granting read on exactly these secrets, and no others."
  value       = module.secrets.read_policy_json
}
