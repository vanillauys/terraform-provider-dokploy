resource "dokploy_application" "example" {
  name           = "web"
  environment_id = dokploy_project.example.production_environment_id

  docker = {
    image = "traefik/whoami:v1.10"
  }

  env = <<-EOT
    PORT=80
  EOT

  # Attach the application to more Docker networks. The attachment applies on
  # the next deploy. Replace the placeholder with a network id from your own
  # server before you uncomment the line.
  # network_ids = ["<dokploy-network-id>"]
}

# From a GitLab project. The provider record holds the OAuth application;
# project_id and path_namespace come from the project's settings page.
resource "dokploy_application" "from_gitlab" {
  name           = "api"
  environment_id = dokploy_project.example.production_environment_id

  gitlab = {
    gitlab_id      = dokploy_gitlab_provider.main.id
    owner          = "my-group"
    repository     = "api"
    branch         = "main"
    project_id     = 12345678
    path_namespace = "my-group/api"
  }

  build = {
    type = "nixpacks"
  }
}

# From a Bitbucket repository. repository_slug is the last part of the
# repository URL.
resource "dokploy_application" "from_bitbucket" {
  name           = "worker"
  environment_id = dokploy_project.example.production_environment_id

  bitbucket = {
    bitbucket_id    = dokploy_bitbucket_provider.main.id
    owner           = "acme"
    repository      = "Worker"
    repository_slug = "worker"
    branch          = "main"
  }
}

# From a Gitea repository, built from a subdirectory.
resource "dokploy_application" "from_gitea" {
  name           = "docs"
  environment_id = dokploy_project.example.production_environment_id

  gitea = {
    gitea_id   = dokploy_gitea_provider.main.id
    owner      = "acme"
    repository = "monorepo"
    branch     = "main"
    build_path = "/docs"
  }
}
