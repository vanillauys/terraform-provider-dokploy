# Look up a backup destination that already exists in Dokploy, by name.
data "dokploy_destination" "backups" {
  name = "cloudflare-r2"
}

# Or by id, when the name is ambiguous or you already hold the id.
data "dokploy_destination" "by_id" {
  id = "V1StGXR8_Z5jdHi6B-myT"
}

# The usual purpose of a lookup: a shared S3 target that exists once and
# that several projects reference.
resource "dokploy_backup" "db" {
  service_id     = dokploy_postgres.main.id
  service_type   = "postgres"
  destination_id = data.dokploy_destination.backups.id
  database       = "app"
  prefix         = "app/"
  schedule       = "0 3 * * *"
}
