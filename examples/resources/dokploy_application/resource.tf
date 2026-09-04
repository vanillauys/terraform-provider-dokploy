resource "dokploy_application" "example" {
  name           = "web"
  environment_id = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]

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
