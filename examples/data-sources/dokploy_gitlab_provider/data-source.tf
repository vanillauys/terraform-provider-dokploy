# Look up a GitLab connection that already exists in Dokploy, by name.
data "dokploy_gitlab_provider" "main" {
  name = "my-group"
}

# Deploy an application from a GitLab project through it.
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.app.production_environment_id

  gitlab = {
    gitlab_id      = data.dokploy_gitlab_provider.main.id
    owner          = "my-group"
    repository     = "api"
    branch         = "main"
    project_id     = 12345678
    path_namespace = "my-group/api"
  }
}
