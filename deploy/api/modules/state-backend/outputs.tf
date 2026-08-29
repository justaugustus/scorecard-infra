output "bucket" {
  description = "Name of the state bucket. Goes in each root module's backend block."
  value       = aws_s3_bucket.state.id
}

output "bucket_arn" {
  description = "ARN of the state bucket."
  value       = aws_s3_bucket.state.arn
}

output "region" {
  description = "Region the state bucket lives in."
  value       = aws_s3_bucket.state.region
}
