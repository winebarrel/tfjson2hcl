resource "aws_iam_policy" "example" {
  policy = <<POLICY
{
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "${aws_s3_bucket.b.arn}",
      "Prefix": "${aws_s3_bucket.b.arn}/*"
    }
  ]
}
POLICY
}
