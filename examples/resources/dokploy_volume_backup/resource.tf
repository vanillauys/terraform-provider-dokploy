# Nightly archive of the uploads volume of an application.
resource "dokploy_volume_backup" "uploads" {
  name            = "web-uploads-nightly"
  service_id      = dokploy_application.web.id
  service_type    = "application"
  volume_name     = "web-uploads"
  prefix          = "volumes/web/"
  cron_expression = "0 4 * * *"
  destination_id  = dokploy_destination.backups.id

  keep_latest_count = 14
}

# Redis has no logical dump in Dokploy, so a volume backup is the only way
# to capture it. dokploy_backup rejects a Redis parent at plan time.
resource "dokploy_volume_backup" "cache" {
  name            = "cache-nightly"
  service_id      = dokploy_redis.cache.id
  service_type    = "redis"
  volume_name     = "cache-data"
  prefix          = "volumes/cache/"
  cron_expression = "30 4 * * *"
  destination_id  = dokploy_destination.backups.id
}
