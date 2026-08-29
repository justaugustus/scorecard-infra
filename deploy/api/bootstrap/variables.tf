variable "region" {
  description = <<-EOT
    AWS region. No default on purpose: the region is an observation, not a
    choice -- it is wherever the result buckets already are, because a service
    in a different region from its buckets loses the S3 gateway endpoint and
    pays NAT egress on every object read. Read it from a capture (A5).
  EOT
  type        = string
}

variable "state_bucket_name" {
  description = "Globally unique name for the OpenTofu state bucket."
  type        = string
}
