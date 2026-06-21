resource "aws_iam_role" "r" {
  name = "r"

  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = []
  })

  inline_policy {
    name = "inline"

    policy = jsonencode({
      Statement = [
        {
          Effect   = "Deny"
          Action   = "*"
          Resource = "*"
        },
      ]
    })
  }
}

resource "aws_instance" "i" {
  user_data = <<-SCRIPT
    #!/bin/bash
    echo not json
  SCRIPT
}
