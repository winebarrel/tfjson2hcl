resource "aws_instance" "with_script" {
  ami = "ami-123456"

  # A shell script is not JSON, so it is left untouched.
  user_data = <<-SCRIPT
    #!/bin/bash
    echo hi
  SCRIPT
}

# Bare ${...} sits outside any JSON string, so the body is not valid JSON and is
# left untouched.
resource "null_resource" "templated" {
  body = <<TPL
{
  "count": ${var.count}
}
TPL
}
