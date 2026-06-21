resource "aws_iam_role" "r" {
  name = "r"

  assume_role_policy = <<POLICY
{"Version": "2012-10-17", "Statement": []}
POLICY

  inline_policy {
    name = "inline"

    policy = <<INLINE
{"Statement": [{"Effect": "Deny", "Action": "*", "Resource": "*"}]}
INLINE
  }
}

resource "aws_instance" "i" {
  user_data = <<-SCRIPT
    #!/bin/bash
    echo not json
  SCRIPT
}
