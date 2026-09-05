# Push messages to a self-hosted Gotify server.
resource "dokploy_gotify_notification" "phone" {
  name       = "phone"
  server_url = "https://gotify.example.com"
  priority   = 8

  app_token_wo         = var.gotify_app_token
  app_token_wo_version = 1

  app_build_error  = true
  server_threshold = true
}
