resource "aws_iam_policy" "escaped" {
  policy = <<POLICY
{
  "Resource": "${aws_s3_bucket.b.arn}/*"
}
POLICY
}
