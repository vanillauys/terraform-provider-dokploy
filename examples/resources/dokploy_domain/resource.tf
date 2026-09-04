resource "dokploy_domain" "app" {
  application_id   = dokploy_application.frontend.id
  host             = "app.example.com"
  port             = 3000
  https            = true
  certificate_type = "letsencrypt"
  enabled          = true
}

# A second hostname for the same application is a second domain resource.
# This one is disabled as an example: `enabled = false` removes the route from
# Traefik but keeps the configuration, so you can enable it again later
# without a new certificate and path setup.
resource "dokploy_domain" "app_www" {
  application_id   = dokploy_application.frontend.id
  host             = "www.app.example.com"
  port             = 3000
  https            = true
  certificate_type = "letsencrypt"
  enabled          = false
}
