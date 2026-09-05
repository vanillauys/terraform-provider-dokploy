# A GitHub Container Registry login. Dokploy pulls private images with it
# and pushes the images it builds when an application references it.
resource "dokploy_registry" "ghcr" {
  name     = "ghcr"
  url      = "ghcr.io"
  username = "my-org-bot"

  # Write-only: Terraform 1.11 or later. Dokploy runs `docker login` on every
  # create and update, so the token must be valid at apply time.
  password_wo         = var.ghcr_token
  password_wo_version = 1

  # Images are pushed as ghcr.io/my-org/<app name>.
  image_prefix = "my-org"
}

# Push the built image of an application to the registry.
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.app.production_environment_id
  registry_id    = dokploy_registry.ghcr.id

  github = {
    github_id  = data.dokploy_github_provider.main.id
    owner      = "my-org"
    repository = "api"
    branch     = "main"
  }
}

# A self-hosted registry on a custom port.
resource "dokploy_registry" "internal" {
  name     = "internal"
  url      = "registry.internal.example.com:5000"
  username = "deploy"
  password = var.internal_registry_password
}
