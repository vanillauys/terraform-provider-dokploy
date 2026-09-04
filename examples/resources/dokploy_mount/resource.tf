# A named Docker volume for the uploads directory of an application.
resource "dokploy_mount" "uploads" {
  service_id   = dokploy_application.web.id
  service_type = "application"

  type        = "volume"
  volume_name = "web-uploads"
  mount_path  = "/app/uploads"
}

# A bind mount from a path on the Dokploy host.
resource "dokploy_mount" "certs" {
  service_id   = dokploy_application.web.id
  service_type = "application"

  type       = "bind"
  host_path  = "/etc/ssl/private/app"
  mount_path = "/certs"
}

# A file that Dokploy writes into the container at deploy time.
resource "dokploy_mount" "config" {
  service_id   = dokploy_application.web.id
  service_type = "application"

  type       = "file"
  mount_path = "/app/config"
  file_path  = "settings.json"
  content    = jsonencode({ log_level = "info" })
}
