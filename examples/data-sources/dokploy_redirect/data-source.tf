data "dokploy_redirect" "example" {
  id = "your-redirect-id"
}

data "dokploy_redirect" "old" {
  application_id = data.dokploy_application.web.id
  regex          = "^/old/(.*)"
}
