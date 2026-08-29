output "cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.this.name
}

output "service_name" {
  description = "ECS service name."
  value       = aws_ecs_service.this.name
}

output "task_role_arn" {
  description = <<-EOT
    The identity the application's own S3 calls authenticate as. Distinct from
    the execution role, which belongs to the ECS agent.
  EOT
  value       = aws_iam_role.task.arn
}

output "execution_role_arn" {
  description = "The ECS agent's role: pulls the image, writes logs, resolves secrets."
  value       = aws_iam_role.execution.arn
}

output "log_group_name" {
  description = "CloudWatch log group for the container."
  value       = aws_cloudwatch_log_group.this.name
}

output "task_definition_arn" {
  description = "Task definition ARN, including its revision."
  value       = aws_ecs_task_definition.this.arn
}
