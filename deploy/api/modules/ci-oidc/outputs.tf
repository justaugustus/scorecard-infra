output "role_arn" {
  description = <<-EOT
    Deploy role ARN. Goes in the workflow's aws-actions/configure-aws-credentials
    step as role-to-assume. No access key is involved and none should be added.
  EOT
  value       = aws_iam_role.deploy.arn
}

output "oidc_provider_arn" {
  description = <<-EOT
    The GitHub OIDC provider. AWS allows one per issuer per account, so the
    second environment applied must pass this as existing_oidc_provider_arn
    with create_oidc_provider = false.
  EOT
  value       = local.oidc_provider_arn
}
