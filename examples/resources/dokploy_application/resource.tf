resource "dokploy_application" "example" {
  name           = "web"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

  docker = {
    image = "traefik/whoami:v1.10"
  }

  env = <<-EOT
    PORT=80
  EOT
}
