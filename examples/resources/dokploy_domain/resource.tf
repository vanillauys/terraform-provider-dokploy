resource "dokploy_domain" "app" {
  application_id   = dokploy_application.frontend.id
  host             = "app.example.com"
  port             = 3000
  https            = true
  certificate_type = "letsencrypt"
}

# A second hostname for the same application is a second domain resource.
resource "dokploy_domain" "app_www" {
  application_id   = dokploy_application.frontend.id
  host             = "www.app.example.com"
  port             = 3000
  https            = true
  certificate_type = "letsencrypt"
}
