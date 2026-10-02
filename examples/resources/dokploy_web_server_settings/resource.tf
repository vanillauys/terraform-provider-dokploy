# Serve the Dokploy dashboard on a domain with a Let's Encrypt certificate,
# and allow two builds at the same time on the Dokploy host. Declare this
# resource once for each Dokploy installation.
resource "dokploy_web_server_settings" "this" {
  host               = "dokploy.example.com"
  https              = true
  certificate_type   = "letsencrypt"
  lets_encrypt_email = "ops@example.com"

  builds_concurrency = 2
  log_cleanup_cron   = "0 3 * * *"
}
