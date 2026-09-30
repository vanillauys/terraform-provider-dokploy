# Back up the Dokploy host every night. Dokploy dumps its own database and
# its configuration directory to the destination.
resource "dokploy_web_server_backup" "nightly" {
  destination_id  = dokploy_destination.backups.id
  cron_expression = "0 3 * * *"
  prefix          = "dokploy/"

  keep_latest_count = 14
}
