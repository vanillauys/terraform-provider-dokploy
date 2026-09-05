# Email through Resend, from a domain that Resend verified.
resource "dokploy_resend_notification" "ops" {
  name         = "ops-resend"
  from_address = "dokploy@example.com"
  to_addresses = ["ops@example.com"]

  api_key_wo         = var.resend_api_key
  api_key_wo_version = 1

  app_build_error = true
  database_backup = true
}
