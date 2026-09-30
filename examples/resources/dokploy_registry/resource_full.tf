# Test the login before the provider writes the record. If the test fails,
# the apply fails and the provider writes nothing. The token is write-only:
# Terraform does not write it to the plan or to the state.
resource "dokploy_registry" "checked" {
  name     = "ghcr-checked"
  url      = "ghcr.io"
  username = "my-org-bot"

  password_wo         = var.ghcr_token
  password_wo_version = 1

  verify_connection = true
}
