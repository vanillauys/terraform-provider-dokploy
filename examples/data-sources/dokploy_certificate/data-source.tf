data "dokploy_certificate" "example" {
  id = "your-certificate-id"
}

data "dokploy_certificate" "wildcard" {
  name = "wildcard-example-com"
}

# A domain that serves the uploaded certificate.
resource "dokploy_domain" "app" {
  host                 = "app.example.com"
  application_id       = dokploy_application.app.id
  https                = true
  certificate_type     = "custom"
  custom_cert_resolver = data.dokploy_certificate.wildcard.name
}
