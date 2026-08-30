variable "region" {
  description = "Same account and region as the serving plane; no compute here to co-locate against."
  type        = string
  default     = "us-east-1"
}
