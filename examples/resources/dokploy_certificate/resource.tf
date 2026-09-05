# Upload a certificate that Traefik serves for a domain. Keep the private key
# out of the state with the write-only companion.
resource "dokploy_certificate" "wildcard" {
  name             = "wildcard-example-com"
  certificate_data = file("${path.module}/certs/wildcard.example.com.pem")

  # Write-only: Terraform 1.11 or later. Change the version when the key file
  # changes, or Terraform does not send the new key.
  private_key_wo         = file("${path.module}/certs/wildcard.example.com.key")
  private_key_wo_version = 1
}

# Use the certificate on a domain.
resource "dokploy_domain" "app" {
  application_id   = dokploy_application.web.id
  host             = "app.example.com"
  https            = true
  certificate_type = "custom"
}

# A certificate that a specific remote server serves. auto_renew and
# server_id cannot change on a stored certificate; a change replaces it.
resource "dokploy_certificate" "worker" {
  name             = "worker-example-com"
  certificate_data = file("${path.module}/certs/worker.pem")
  private_key      = file("${path.module}/certs/worker.key")
  auto_renew       = false
  server_id        = dokploy_server.worker.id
}
