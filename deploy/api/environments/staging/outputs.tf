output "certificate_validation_records" {
  description = <<-EOT
    Create these in Netlify DNS. The apply blocks until they resolve, which is
    intentional -- a stalled apply is a visible gate rather than a silent
    prerequisite. See the two-phase procedure in modules/edge/main.tf.
  EOT
  value       = module.edge.certificate_validation_records
}

output "alb_dns_name" {
  description = <<-EOT
    Point origin_hostname's CNAME at this in Netlify. Then point the Fastly
    STAGING backend at origin_hostname -- not at this name, which belongs to
    this particular load balancer rather than to the service.
  EOT
  value       = module.edge.alb_dns_name
}

output "origin_hostname" {
  description = "What the Fastly staging backend should be set to, once DNS resolves."
  value       = var.origin_hostname
}

output "nat_egress_ips" {
  description = "Stable outbound addresses, should GitHub or GitLab ever need an allowlist."
  value       = module.network.nat_public_ips
}

output "deploy_role_arn" {
  description = "For the workflow's configure-aws-credentials step. No access key is involved."
  value       = module.ci_oidc.role_arn
}

output "oidc_provider_arn" {
  description = <<-EOT
    Pass to production as existing_oidc_provider_arn with
    create_oidc_provider = false: AWS permits one provider per issuer per
    account, and staging creates it.
  EOT
  value       = module.ci_oidc.oidc_provider_arn
}

output "log_group_name" {
  description = "Where the container logs land."
  value       = module.service.log_group_name
}
