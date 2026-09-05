# Register the OAuth2 application that you created in Gitea
# (Settings > Applications). The default redirect_uri matches the provider's
# endpoint; register the same URI in Gitea.
resource "dokploy_gitea_provider" "main" {
  name      = "gitea"
  gitea_url = "https://gitea.example.com"
  client_id = var.gitea_client_id

  # Write-only: Terraform 1.11 or later. Change the version to send a new secret.
  client_secret_wo         = var.gitea_client_secret
  client_secret_wo_version = 1
}

# After the apply, open Git > Gitea in Dokploy and authorize the application
# once. Until then the provider holds no access token.
