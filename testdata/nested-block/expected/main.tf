resource "aws_ecs_task_definition" "svc" {
  family = "svc"

  container_definitions = jsonencode([
    {
      name      = "app"
      image     = "nginx:latest"
      essential = true
      portMappings = [
        {
          containerPort = 80
        },
      ]
    },
  ])
}

locals {
  cfg = jsonencode({
    a = 1
    nested = {
      b = true
      list = [
        1,
        2,
        3,
      ]
    }
  })
}
