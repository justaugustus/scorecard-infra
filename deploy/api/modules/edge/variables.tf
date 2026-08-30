variable "name" {
  description = "Name prefix for the load balancer and its friends."
  type        = string
}

variable "vpc_id" {
  description = "VPC the target group and security group live in."
  type        = string
}

variable "public_subnet_ids" {
  description = "Public subnets for the load balancer. At least two AZs."
  type        = list(string)
}

variable "origin_hostname" {
  description = <<-EOT
    The hostname the CDN treats as its origin, e.g. origin-staging.scorecard.dev.

    This is the single most load-bearing string in the deployment. Fastly is
    pinned to it, the production cutover is one backend field pointing here, and
    rollback is restoring that field. It must be a hostname with a certificate
    Fastly can verify -- never a bare IP, which would require overriding SNI and
    the host header plus either a certificate issued for an IP or disabled
    origin verification (A10).
  EOT
  type        = string
}

variable "target_port" {
  description = "Port the container listens on. api/Dockerfile: --port=8080."
  type        = number
  default     = 8080
}

variable "health_check_path" {
  description = <<-EOT
    Path the load balancer probes.

    "/" on purpose: the shipping API exposes NO health endpoint. Its contract is
    two routes -- /projects/{platform}/{org}/{repo} and .../badge -- with no
    /health, no /readyz, and no HEALTHCHECK in its Dockerfile. (The
    health-endpoint capability belongs to the provider-agnostic server in
    internal/, which is not what deploys.) "/" actually returns 200 with the
    generated Swagger UI, and that 200 is the signal: the process is up,
    listening, and routing. Tightening the matcher to require exactly 200 would
    couple target health to the Swagger UI assets being served, which is the
    coupling this variable is meant to avoid -- hence health_check_matcher
    below stays a wide range rather than narrowing to match this path.
  EOT
  type        = string
  default     = "/"
}

variable "health_check_matcher" {
  description = <<-EOT
    HTTP codes counted as healthy. Any non-5xx, deliberately.

    This checks LIVENESS, not correctness, and that is the right scope for a
    load balancer: its question is "should this target receive traffic", not "is
    the system working". Probing a real /projects path instead would fold S3
    availability into target health -- and because every target would fail the
    same probe at the same moment, a transient S3 problem would drain the whole
    pool and turn a degraded service into an unreachable one.

    Cloud Run has no application health check today either, so this is also the
    closer match to the behavior being migrated.
  EOT
  type        = string
  default     = "200-499"
}

variable "allowed_ingress_cidrs" {
  description = <<-EOT
    Who may reach the load balancer.

    Open by default, which matches what is being replaced: the Cloud Run service
    runs with ingress "all". Narrowing this to Fastly's published ranges is a
    genuine improvement and is deliberately NOT bundled here -- this migration's
    acceptance claim is that behavior did not change, and a posture change
    smuggled in alongside it is indistinguishable from a regression when
    something breaks. Do it as its own change, once traffic is stable.
  EOT
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "deregistration_delay" {
  description = <<-EOT
    Seconds to drain a target before removing it. Well under the 300s default:
    responses are short and stateless, and a long drain mostly makes deploys
    slow enough that people stop doing them.
  EOT
  type        = number
  default     = 30
}

variable "tls_policy" {
  description = "ALB security policy. TLS 1.2 floor with 1.3 available."
  type        = string
  default     = "ELBSecurityPolicy-TLS13-1-2-2021-06"
}

variable "tags" {
  description = "Tags applied to every resource, so anything untagged reads as drift."
  type        = map(string)
  default     = {}
}
