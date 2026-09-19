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

# A database running inside a dokploy_compose service needs
# compose_database_type as well: service_type alone cannot say both "the
# parent is a compose service" and "the engine inside it is mariadb". It also
# needs the credentials of the dump command, because a compose service has no
# database record that Dokploy can read them from: the user for postgres, the
# user and the password for mariadb and mongo, the root password for mysql.
# The write-only companion keeps the password out of the state.
resource "dokploy_backup" "nextcloud_db" {
  service_id            = dokploy_compose.nextcloud.id
  service_type          = "compose"
  compose_database_type = "mariadb"
  service_name          = "nextcloud-db"
  database              = "nextcloud"
  prefix                = "/nextcloud"
  cron_expression       = "0 0 * * *"
  destination_id        = dokploy_destination.backups.id

  compose_database_user                = "nextcloud"
  compose_database_password_wo         = var.nextcloud_db_password
  compose_database_password_wo_version = 1

  keep_latest_count = 10
}
