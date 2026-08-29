variable "name" {
  description = "Name prefix for every resource in this VPC."
  type        = string
}

variable "vpc_cidr" {
  description = <<-EOT
    CIDR for the VPC. Must not overlap the default VPC (172.31.0.0/16) if the
    two are ever peered, which is why the default here is in 10/8.
  EOT
  type        = string
  default     = "10.20.0.0/16"
}

variable "availability_zones" {
  description = <<-EOT
    AZs to spread across. Two is the floor for an ALB, which requires subnets in
    at least two. More AZs cost nothing by themselves but multiply NAT gateways
    if single_nat_gateway is false.
  EOT
  type        = list(string)

  validation {
    condition     = length(var.availability_zones) >= 2
    error_message = "An ALB requires subnets in at least two availability zones."
  }
}

variable "single_nat_gateway" {
  description = <<-EOT
    One NAT gateway for all private subnets, rather than one per AZ.

    True by default because NAT is the largest fixed line item after compute
    (~$33/month each) and this service's egress is Sigstore, GitHub, and Fastly
    purges -- not bulk transfer. The Elastic IP quota on this account is 5,
    shared with everything else, which is a second reason not to spend them
    per-AZ by default.

    The trade is real: with one gateway, losing its AZ costs egress from the
    others. Inbound traffic and S3 reads are unaffected -- the ALB is
    multi-AZ and S3 goes via the gateway endpoint, not NAT.
  EOT
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags applied to every resource, so anything untagged reads as drift."
  type        = map(string)
  default     = {}
}
