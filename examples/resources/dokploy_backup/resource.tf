resource "dokploy_backup" "db_nightly" {
  service_id     = dokploy_postgres.db.id
  service_type   = "postgres"
  database       = "vanillauys"
  prefix         = "backups/vanillauys/"
  schedule       = "0 3 * * *"
  destination_id = dokploy_destination.backups.id

  keep_latest_count = 30
}

# Redis has no logical dump in Dokploy. This resource rejects it at plan
# time; use dokploy_volume_backup instead.
