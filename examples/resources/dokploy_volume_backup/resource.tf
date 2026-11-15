# Nightly archive of the uploads volume of an application. Dokploy accepts
# only a volume that the service mounts, so the backup references the mount.
resource "dokploy_mount" "uploads" {
  service_id   = dokploy_application.web.id
  service_type = "application"
  type         = "volume"
  volume_name  = "web-uploads"
  mount_path   = "/app/uploads"
}

resource "dokploy_volume_backup" "uploads" {
  name            = "web-uploads-nightly"
  service_id      = dokploy_application.web.id
  service_type    = "application"
  volume_name     = dokploy_mount.uploads.volume_name
  prefix          = "volumes/web/"
  cron_expression = "0 4 * * *"
  destination_id  = dokploy_destination.backups.id

  keep_latest_count = 14
}

# Redis has no logical dump in Dokploy, so a volume backup is the only way
# to capture it. dokploy_backup rejects a Redis parent at plan time. Dokploy
# creates the data volume <app_name>-data with each database.
resource "dokploy_volume_backup" "cache" {
  name            = "cache-nightly"
  service_id      = dokploy_redis.cache.id
  service_type    = "redis"
  volume_name     = "${dokploy_redis.cache.app_name}-data"
  prefix          = "volumes/cache/"
  cron_expression = "30 4 * * *"
  destination_id  = dokploy_destination.backups.id
}
