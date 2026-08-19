resource "dokploy_domain" "app" {
  application_id   = dokploy_application.frontend.id
  host             = "app.example.com"
  port             = 3000
  https            = true
  certificate_type = "letsencrypt"
  enabled          = true
}

# A second hostname for the same application is a second domain resource.
# Disabled here as an example: `enabled = false` removes the route from
# Traefik but keeps the configuration, so it can be re-enabled later without
# re-entering certificates and paths.
resource "dokploy_domain" "app_www" {
  application_id   = dokploy_application.frontend.id
  host             = "www.app.example.com"
  port             = 3000
  https            = true
  certificate_type = "letsencrypt"
  enabled          = false
}
