resource "aws_iam_policy" "example" {
  name = "example"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
        ]
        Resource = "${aws_s3_bucket.b.arn}/*"
      },
    ]
  })
}
