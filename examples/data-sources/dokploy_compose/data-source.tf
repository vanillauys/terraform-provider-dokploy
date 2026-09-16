data "dokploy_compose" "example" {
  id = "your-compose-id"
}

# By name, within an environment.
data "dokploy_compose" "stalwart" {
  environment_id = data.dokploy_environment.production.id
  name           = "stalwart"
}

# Attach a domain to a stack that Terraform does not manage.
resource "dokploy_domain" "mail" {
  host         = "mail.example.com"
  compose_id   = data.dokploy_compose.stalwart.id
  service_name = "stalwart"
  port         = 8080
  https        = true
}
