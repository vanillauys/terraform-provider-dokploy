data "dokploy_security" "example" {
  id = "your-security-id"
}

data "dokploy_security" "admin" {
  application_id = data.dokploy_application.web.id
  username       = "admin"
}
