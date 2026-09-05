# Look up a Gitea connection that already exists in Dokploy, by name.
data "dokploy_gitea_provider" "main" {
  name = "gitea"
}

# Deploy an application from a Gitea repository through it.
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.app.production_environment_id

  gitea = {
    gitea_id   = data.dokploy_gitea_provider.main.id
    owner      = "acme"
    repository = "api"
    branch     = "main"
  }
}
