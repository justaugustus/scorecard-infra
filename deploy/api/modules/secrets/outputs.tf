output "secret_arns" {
  description = "Secret ARNs keyed by short name. The ECS task definition references these."
  value       = { for k, s in aws_secretsmanager_secret.this : k => s.arn }
}

output "read_policy_json" {
  description = "IAM policy granting read on exactly these secrets, and no others."
  value       = data.aws_iam_policy_document.read.json
}
