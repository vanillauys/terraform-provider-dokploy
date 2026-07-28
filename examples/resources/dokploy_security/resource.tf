# Basic-auth in front of a staging application.
resource "dokploy_security" "staging_gate" {
  application_id = dokploy_application.staging.id
  username       = "preview"
  password       = var.staging_password
}
