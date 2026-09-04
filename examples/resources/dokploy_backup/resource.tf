resource "dokploy_backup" "db_nightly" {
  service_id      = dokploy_postgres.db.id
  service_type    = "postgres"
  database        = "app"
  prefix          = "backups/app/"
  cron_expression = "0 3 * * *"
  destination_id  = dokploy_destination.backups.id

  keep_latest_count = 30
}

# Redis has no logical dump in Dokploy. This resource rejects a Redis parent
# at plan time. Use dokploy_volume_backup instead.
