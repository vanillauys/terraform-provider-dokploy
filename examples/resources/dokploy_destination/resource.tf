# Cloudflare R2 is S3-compatible and works as a Dokploy backup destination.
resource "dokploy_destination" "backups" {
  name              = "app-backups"
  provider_name     = "Cloudflare"
  endpoint          = "https://${var.r2_account_id}.r2.cloudflarestorage.com"
  bucket            = "app-backups"
  region            = "WEUR"
  access_key        = var.r2_access_key
  secret_access_key = var.r2_secret_access_key
}
