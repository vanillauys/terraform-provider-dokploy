# Back up the Dokploy host to an S3 bucket. The destination and the backup
# are two resources. The backup refers to the destination by id.
resource "dokploy_destination" "host_backups" {
  name          = "host-backups"
  provider_name = "AWS"
  endpoint      = "https://s3.eu-west-1.amazonaws.com"
  bucket        = "dokploy-host-backups"
  region        = "eu-west-1"

  access_key_wo                = var.s3_access_key
  access_key_wo_version        = 1
  secret_access_key_wo         = var.s3_secret_access_key
  secret_access_key_wo_version = 1
}

resource "dokploy_web_server_backup" "host" {
  destination_id  = dokploy_destination.host_backups.id
  cron_expression = "0 3 * * *"
  prefix          = "dokploy/"

  keep_latest_count      = 14
  include_encryption_key = true
}
