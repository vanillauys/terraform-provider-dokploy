# Look up the GitHub App by name instead of an opaque id.
data "dokploy_github_provider" "main" {
  name = "my-org"
}

resource "dokploy_application" "web" {
  name           = "web"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

  github = {
    owner      = "my-org"
    repository = "my-app"
    branch     = "master"
    # Use `id`, not `git_provider_id`: an application references the
    # GitHub-specific record.
    github_id = data.dokploy_github_provider.main.id
  }
}
