data "dokploy_port" "example" {
  id = "your-port-id"
}

data "dokploy_port" "web" {
  application_id = data.dokploy_application.web.id
  published_port = 8080
}
