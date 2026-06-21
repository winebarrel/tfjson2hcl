# tfjson2hcl

[![CI](https://github.com/winebarrel/tfjson2hcl/actions/workflows/ci.yml/badge.svg)](https://github.com/winebarrel/tfjson2hcl/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/winebarrel/tfjson2hcl/branch/main/graph/badge.svg)](https://codecov.io/gh/winebarrel/tfjson2hcl)
[![AI Generated](https://img.shields.io/badge/AI%20Generated-Claude-orange?logo=anthropic)](https://claude.ai/claude-code)

`tfjson2hcl` rewrites Terraform `*.tf` files, converting heredoc-embedded JSON into `jsonencode({...})` expressions. It uses [json2hcl](https://github.com/winebarrel/json2hcl) as a library to turn the JSON into HCL.

Heredocs whose content is not valid JSON (shell scripts, templates with bare `${...}` outside a string, ...) are left untouched.

## Installation

```
brew install winebarrel/tfjson2hcl/tfjson2hcl
```

## Usage

```
Usage: tfjson2hcl [<dir>] [flags]

Convert heredoc JSON in *.tf files into jsonencode({...}) expressions.

Arguments:
  [<dir>]    Directory containing *.tf files (default: ".").

Flags:
  -h, --help        Show help.
  -i, --in-place    Write changes back to files instead of stdout.
  -e, --escape      Escape ${...} and %{...} to $${...} / %%{...} instead of
                    keeping them as template interpolations.
      --version
```

By default the rewritten files are printed to stdout. Pass `-i` to overwrite files on disk. Only files that actually change are emitted.

## Example

```hcl
# main.tf
resource "aws_iam_policy" "example" {
  policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "${aws_s3_bucket.b.arn}/*",
      "Bucket": "${aws_s3_bucket.b.arn}"
    }
  ]
}
POLICY
}
```

```sh
tfjson2hcl -i .
```

```hcl
# main.tf (rewritten)
resource "aws_iam_policy" "example" {
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = "s3:GetObject"
        Resource = "${aws_s3_bucket.b.arn}/*"
        Bucket   = aws_s3_bucket.b.arn
      },
    ]
  })
}
```

## How it works

1. `*.tf` files in the target directory are parsed with `hclwrite`.
2. Every attribute whose value is a heredoc is examined. The raw heredoc body is fed to `json2hcl`. If it parses as JSON, the attribute is rewritten to `jsonencode(<converted HCL>)`; otherwise it is left as-is.
3. Template interpolations inside JSON strings (`${...}`) are preserved by default so the resulting `jsonencode` behaves like the original heredoc. A string that is exactly one interpolation (`"${foo.bar}"`) is unwrapped to the bare expression (`foo.bar`), matching `terraform fmt`. Strings with surrounding literal text (`"${x}/*"`, `"${a}${b}"`) keep their quotes.

## Interpolation handling

By default `${...}` and `%{...}` are kept as Terraform template sequences, because that is how they behaved inside the original heredoc. Pass `-e` / `--escape` to emit them as literal text (`$${...}` / `%%{...}`) instead; in that mode nothing is unwrapped, since the sequences are no longer interpolations.

## Limitations

- Only `*.tf` files directly in the target directory are scanned. Subdirectories are not recursed.
- A heredoc must hold a single, complete JSON document. Heredocs containing Terraform interpolation outside of a JSON string (`"count": ${var.n}`) are not valid JSON and are skipped.
- Only heredocs that are the direct value of an attribute (`policy = <<JSON`) are converted. A heredoc nested inside an object or map expression (`triggers = { body = <<JSON ... }`) is left untouched.
- A JSON string that contains a Terraform template directive (`"%{ if x }..."`) cannot be expressed as a `jsonencode` string and is left untouched.
