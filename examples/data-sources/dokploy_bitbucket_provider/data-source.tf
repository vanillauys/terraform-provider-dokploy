# Look up a Bitbucket connection that already exists in Dokploy, by name.
data "dokploy_bitbucket_provider" "main" {
  name = "acme"
}

# Deploy an application from a Bitbucket repository through it.
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.app.production_environment_id

  bitbucket = {
    bitbucket_id    = data.dokploy_bitbucket_provider.main.id
    owner           = "acme"
    repository      = "api"
    repository_slug = "api"
    branch          = "main"
  }
}
