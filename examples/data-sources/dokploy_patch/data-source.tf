data "dokploy_patch" "example" {
  id = "your-patch-id"
}

# Look a patch up by the service and the file path.
data "dokploy_patch" "config" {
  application_id = data.dokploy_application.web.id
  file_path      = "config/app.yaml"
}
