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
