# The API-token shape: an Atlassian account email with an API token.
resource "dokploy_bitbucket_provider" "main" {
  name           = "acme"
  email          = "deploy-bot@example.com"
  workspace_name = "acme"

  # Write-only: Terraform 1.11 or later. Change the version to rotate the token.
  api_token_wo         = var.bitbucket_api_token
  api_token_wo_version = 1
}

# The app-password shape, which Atlassian has deprecated. Keep it only for
# an existing setup.
resource "dokploy_bitbucket_provider" "legacy" {
  name         = "acme-legacy"
  username     = "deploy-bot"
  app_password = var.bitbucket_app_password
}
