# Replace a file in the repository of an application before each build.
resource "dokploy_patch" "config" {
  application_id = dokploy_application.web.id
  file_path      = "config/app.yaml"
  content        = file("${path.module}/app.yaml")
}

# Remove a file from the repository of a compose service before each build.
resource "dokploy_patch" "no_override" {
  compose_id = dokploy_compose.stack.id
  file_path  = "docker-compose.override.yml"
  type       = "delete"
}
