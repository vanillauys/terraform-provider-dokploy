resource "dokploy_application" "example" {
  name           = "web"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

  docker = {
    image = "traefik/whoami:v1.10"
  }

  env = <<-EOT
    PORT=80
  EOT

  # Attach to extra Docker networks. Applied on the next deploy. A bare id
  # would not apply for other users, so this stays commented out - replace
  # it with a real network id from your own instance before you uncomment it.
  # network_ids = ["<dokploy-network-id>"]
}
