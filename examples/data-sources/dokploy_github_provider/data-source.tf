# Look up the GitHub App by name instead of an opaque id.
data "dokploy_github_provider" "main" {
  name = "my-org"
}

resource "dokploy_application" "web" {
  name           = "web"
  environment_id = dokploy_project.example.production_environment_id

  github = {
    owner      = "my-org"
    repository = "my-app"
    branch     = "master"
    # Use `id`, not `git_provider_id`: an application references the
    # GitHub-specific record.
    github_id = data.dokploy_github_provider.main.id
  }
}
