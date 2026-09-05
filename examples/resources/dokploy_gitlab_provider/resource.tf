# Register the OAuth application that you created in GitLab
# (User Settings > Applications, or a group application). GitLab needs the
# redirect URI that Dokploy reports back; the default matches the provider's
# endpoint, so most setups leave redirect_uri unset.
resource "dokploy_gitlab_provider" "main" {
  name           = "my-group"
  application_id = var.gitlab_oauth_application_id
  group_name     = "my-group"

  # Write-only: Terraform 1.11 or later. Change the version to send a new secret.
  secret_wo         = var.gitlab_oauth_secret
  secret_wo_version = 1
}

# After the apply, open Git > GitLab in Dokploy and authorize the
# application once. Until then the provider holds no access token.

# A self-hosted GitLab on a private network: gitlab_internal_url is the
# address that the Dokploy server uses to reach it.
resource "dokploy_gitlab_provider" "internal" {
  name                = "gitlab-internal"
  gitlab_url          = "https://gitlab.example.com"
  gitlab_internal_url = "http://10.0.0.5"
  application_id      = var.internal_oauth_application_id
  secret              = var.internal_oauth_secret
}
