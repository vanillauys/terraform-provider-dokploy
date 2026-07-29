# Look the GitHub App up by name instead of pasting an opaque id.
data "dokploy_github_provider" "main" {
  name = "vnly-io-dokploy"
}

resource "dokploy_application" "web" {
  name           = "web"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

  github = {
    owner      = "vanillauys"
    repository = "vanillauys-app"
    branch     = "master"
    # NOTE: `id`, not `git_provider_id` — an application references the
    # GitHub-specific record.
    github_id = data.dokploy_github_provider.main.id
  }
}
