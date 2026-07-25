resource "dokploy_application" "example" {
  name           = "web"
  environment_id = dokploy_project.example.environments[0].id

  docker = {
    image = "traefik/whoami:v1.10"
  }

  env = <<-EOT
    PORT=80
  EOT
}
