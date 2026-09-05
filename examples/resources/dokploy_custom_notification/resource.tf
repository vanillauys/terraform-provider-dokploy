# Send each event as a JSON POST to an endpoint of your own, with an
# authorization header.
resource "dokploy_custom_notification" "pipeline" {
  name     = "pipeline"
  endpoint = "https://hooks.example.com/dokploy"

  headers = {
    "Authorization" = "Bearer ${var.hooks_token}"
    "X-Source"      = "dokploy"
  }

  app_deploy      = true
  app_build_error = true
  database_backup = true
}
