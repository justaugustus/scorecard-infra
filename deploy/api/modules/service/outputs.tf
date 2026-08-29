output "cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.this.name
}

output "cluster_arn" {
  description = "ECS cluster ARN. Scopes the deploy role's rollout-observation permissions."
  value       = aws_ecs_cluster.this.arn
}

output "service_name" {
  description = "ECS service name."
  value       = aws_ecs_service.this.name
}

output "service_arn" {
  description = "ECS service ARN. The one service the deploy role may update."
  value       = aws_ecs_service.this.id
}

output "task_definition_family" {
  description = "Task definition family the deploy role registers new revisions of."
  value       = aws_ecs_task_definition.this.family
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
