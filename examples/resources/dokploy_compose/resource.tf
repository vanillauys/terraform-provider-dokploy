resource "dokploy_project" "example" {
  name = "example"
}

# Inline compose file. Dokploy fetches nothing: the YAML below is the source.
resource "dokploy_compose" "inline" {
  name           = "inline-stack"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
  description    = "Defined entirely in Terraform"

  raw = {
    compose_file = <<-YAML
      services:
        web:
          image: nginx:alpine
          ports:
            - "8080:80"
    YAML
  }
}

# From a GitHub App repository. github_id comes from the data source, not
# from a hardcoded opaque id.
data "dokploy_github_provider" "main" {
  name = "my-org"
}

resource "dokploy_compose" "from_github" {
  name           = "github-stack"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

  github = {
    repository = "my-stack"
    owner      = "my-org"
    branch     = "main"
    github_id  = data.dokploy_github_provider.main.id
  }

  compose_path = "./deploy/docker-compose.yml"
  auto_deploy  = true
  trigger_type = "push"
  watch_paths  = ["deploy/**"]
}

# From a plain git remote, run as a Docker Swarm stack.
resource "dokploy_compose" "from_git" {
  name           = "git-stack"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
  compose_type   = "stack"

  git = {
    url    = "https://github.com/my-org/my-stack.git"
    branch = "main"
  }

  env = <<-ENV
    LOG_LEVEL=info
    REGION=eu-west-1
  ENV
}

# Route traffic to one service inside the stack.
resource "dokploy_domain" "web" {
  host         = "example.com"
  compose_id   = dokploy_compose.inline.id
  service_name = "web"
  port         = 80
  https        = true
}
