resource "aws_ecs_task_definition" "svc" {
  family = "svc"

  container_definitions = <<DEF
[
  {
    "name": "app",
    "image": "nginx:latest",
    "essential": true,
    "portMappings": [{"containerPort": 80}]
  }
]
DEF
}

locals {
  cfg = <<JSON
{"a": 1, "nested": {"b": true, "list": [1, 2, 3]}}
JSON
}
