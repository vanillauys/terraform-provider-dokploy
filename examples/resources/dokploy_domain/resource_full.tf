# A domain with a Let's Encrypt certificate on an application. Point the DNS
# record of the host at the Dokploy server before the apply. Traefik requests
# the certificate on the first request to the host.
resource "dokploy_application" "web" {
  name           = "web"
  environment_id = dokploy_project.example.production_environment_id

  docker = {
    image = "traefik/whoami:v1.10"
  }
}

resource "dokploy_domain" "web" {
  application_id   = dokploy_application.web.id
  host             = "web.example.com"
  port             = 80
  https            = true
  certificate_type = "letsencrypt"
}
