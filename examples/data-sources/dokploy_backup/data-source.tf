data "dokploy_backup" "example" {
  id = "your-backup-id"
}

data "dokploy_backup" "nightly" {
  service_id   = data.dokploy_postgres.main.id
  service_type = "postgres"
  prefix       = "nightly"
}
