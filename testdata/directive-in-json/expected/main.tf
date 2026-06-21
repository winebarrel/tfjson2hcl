# The JSON is valid, but one string value contains a Terraform template
# directive (%{ ... }). Converting it would produce an invalid directive, so the
# heredoc is left untouched.
resource "null_resource" "directive" {
  body = <<JSON
{"a": "%{ if x }y%{ endif }", "b": "plain"}
JSON
}
