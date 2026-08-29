output "certificate_validation_records" {
  description = "Create in Netlify DNS; the apply blocks until they resolve."
  value       = module.edge.certificate_validation_records
}

output "alb_dns_name" {
  description = <<-EOT
    Point origin_hostname's CNAME at this in Netlify.

    Do NOT put this name in the Fastly backend. Point Fastly at
    origin_hostname, so replacing the load balancer later is a DNS change rather
    than a production cutover.
  EOT
  value       = module.edge.alb_dns_name
}

output "origin_hostname" {
  description = <<-EOT
    The cutover: set the Fastly production backend to this. Rollback is
    restoring the previous value, which is why the old origin should not be
    decommissioned until the hold period ends.
  EOT
  value       = var.origin_hostname
}

output "nat_egress_ips" {
  description = "Stable outbound addresses, should GitHub or GitLab ever need an allowlist."
  value       = module.network.nat_public_ips
}

output "deploy_role_arn" {
  description = "For the workflow's configure-aws-credentials step."
  value       = module.ci_oidc.role_arn
}

output "log_group_name" {
  description = "Where the container logs land."
  value       = module.service.log_group_name
}
