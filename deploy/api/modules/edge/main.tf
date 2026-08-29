# The origin: an ALB with an ACM certificate, which is what Fastly connects to.
#
# OpenTofu owns all of it. The origin's lifecycle has to be independent of any
# workload object, because the cutover is a single Fastly backend field pointing
# here and rollback is restoring that field -- so nothing should be able to
# change or destroy the hostname as a side effect of a deployment (A9).
#
# TWO-PHASE APPLY. The zones are on Netlify DNS, so the certificate's validation
# records cannot be created from here. aws_acm_certificate_validation waits for
# them, which is intentional -- a stalled apply is a visible gate rather than a
# silent prerequisite -- but it means the first run goes:
#
#   tofu apply -target=module.edge.aws_acm_certificate.this
#   tofu output -json certificate_validation_records   # create these in Netlify
#   tofu apply                                         # completes once validated
#
# Then point origin_hostname's own CNAME at alb_dns_name, also in Netlify.

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

resource "aws_acm_certificate" "this" {
  domain_name       = var.origin_hostname
  validation_method = "DNS"

  # Replacing a certificate in place would briefly leave the listener without
  # one, and the listener is the origin.
  lifecycle {
    create_before_destroy = true
  }

  tags = merge(var.tags, { Name = var.origin_hostname })
}

resource "aws_acm_certificate_validation" "this" {
  certificate_arn = aws_acm_certificate.this.arn

  validation_record_fqdns = [
    for o in aws_acm_certificate.this.domain_validation_options :
    o.resource_record_name
  ]
}

# --- Security groups --------------------------------------------------------

resource "aws_security_group" "alb" {
  name        = "${var.name}-alb"
  description = "Public entry point. The only thing in this VPC reachable from the internet."
  vpc_id      = var.vpc_id

  tags = merge(var.tags, { Name = "${var.name}-alb" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "alb_https" {
  for_each = toset(var.allowed_ingress_cidrs)

  security_group_id = aws_security_group.alb.id
  description       = "HTTPS from the CDN"
  cidr_ipv4         = each.value
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

# Present only to redirect. Fastly is configured for HTTPS; this catches
# anything that arrives on 80 and sends it back rather than quietly serving it.
resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  for_each = toset(var.allowed_ingress_cidrs)

  security_group_id = aws_security_group.alb.id
  description       = "HTTP, redirected to HTTPS"
  cidr_ipv4         = each.value
  from_port         = 80
  to_port           = 80
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "alb_to_targets" {
  security_group_id            = aws_security_group.alb.id
  description                  = "To the tasks. Narrowed to the task security group, not the VPC."
  referenced_security_group_id = aws_security_group.tasks.id
  from_port                    = var.target_port
  to_port                      = var.target_port
  ip_protocol                  = "tcp"
}

# Created here rather than in the service module so that the ALB and task rules
# can reference each other without the two modules forming a dependency cycle.
resource "aws_security_group" "tasks" {
  name        = "${var.name}-tasks"
  description = "The API tasks. Reachable only from the load balancer."
  vpc_id      = var.vpc_id

  tags = merge(var.tags, { Name = "${var.name}-tasks" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "tasks_from_alb" {
  security_group_id            = aws_security_group.tasks.id
  description                  = "From the load balancer only. No path in from the internet."
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = var.target_port
  to_port                      = var.target_port
  ip_protocol                  = "tcp"
}

# Outbound: S3 via the gateway endpoint, plus Sigstore, GitHub and Fastly on the
# publish path. Those are arbitrary internet destinations behind NAT, so this
# cannot be usefully narrowed by CIDR.
resource "aws_vpc_security_group_egress_rule" "tasks_all" {
  security_group_id = aws_security_group.tasks.id
  description       = "Sigstore, GitHub, Fastly purges, and S3"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

# --- Load balancer ----------------------------------------------------------

resource "aws_lb" "this" {
  name               = var.name
  load_balancer_type = "application"
  internal           = false
  subnets            = var.public_subnet_ids
  security_groups    = [aws_security_group.alb.id]

  drop_invalid_header_fields = true

  # The origin is the one thing the cutover and the rollback both depend on.
  enable_deletion_protection = true

  tags = merge(var.tags, { Name = var.name })
}

resource "aws_lb_target_group" "this" {
  name        = var.name
  port        = var.target_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip" # required for awsvpc networking, which Fargate mandates

  deregistration_delay = var.deregistration_delay

  health_check {
    enabled  = true
    path     = var.health_check_path
    matcher  = var.health_check_matcher
    protocol = "HTTP"

    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(var.tags, { Name = var.name })
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = var.tls_policy

  # The validated ARN, not the certificate's own, so the listener cannot come up
  # holding a certificate Fastly would refuse.
  certificate_arn = aws_acm_certificate_validation.this.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this.arn
  }

  tags = var.tags
}

resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }

  tags = var.tags
}
