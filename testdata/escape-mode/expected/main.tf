resource "aws_iam_policy" "escaped" {
  policy = jsonencode({
    Resource = "$${aws_s3_bucket.b.arn}/*"
  })
}
