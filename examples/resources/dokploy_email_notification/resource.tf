# Email the operations team about deploys and failed builds.
resource "dokploy_email_notification" "ops" {
  name         = "ops-mail"
  smtp_server  = "smtp.example.com"
  smtp_port    = 587
  username     = "dokploy@example.com"
  from_address = "dokploy@example.com"
  to_addresses = ["ops@example.com", "dev@example.com"]

  # Write-only: Terraform 1.11 or later.
  password_wo         = var.smtp_password
  password_wo_version = 1

  app_deploy      = true
  app_build_error = true
}
