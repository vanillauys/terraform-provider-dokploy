data "dokploy_registry" "example" {
  id = "your-registry-id"
}

data "dokploy_registry" "ghcr" {
  name = "ghcr"
}

# Push the built images to the registry.
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = data.dokploy_environment.production.id
  registry_id    = data.dokploy_registry.ghcr.id

  git = {
    url    = "https://github.com/example/api.git"
    branch = "main"
  }
}
