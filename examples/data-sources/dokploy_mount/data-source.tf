data "dokploy_mount" "example" {
  id = "your-mount-id"
}

# Look a mount up by the service and the path in the container.
data "dokploy_mount" "data" {
  service_id   = data.dokploy_application.web.id
  service_type = "application"
  mount_path   = "/data"
}
