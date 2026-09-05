# Publish to a protected topic on ntfy.sh.
resource "dokploy_ntfy_notification" "phone" {
  name       = "phone"
  server_url = "https://ntfy.sh"
  topic      = "my-org-dokploy"
  priority   = 4

  access_token_wo         = var.ntfy_access_token
  access_token_wo_version = 1

  app_deploy      = true
  app_build_error = true
}

# A public topic needs no token.
resource "dokploy_ntfy_notification" "public" {
  name       = "public"
  server_url = "https://ntfy.sh"
  topic      = "my-org-dokploy-public"

  dokploy_restart = true
}
