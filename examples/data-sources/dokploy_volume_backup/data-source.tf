data "dokploy_volume_backup" "example" {
  id = "your-volume-backup-id"
}

data "dokploy_volume_backup" "data" {
  service_id   = data.dokploy_application.web.id
  service_type = "application"
  name         = "data-volume"
}
