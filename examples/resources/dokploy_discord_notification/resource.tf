# Post every backup result to a Discord channel, as a plain message.
resource "dokploy_discord_notification" "backups" {
  name        = "backups"
  webhook_url = var.discord_webhook_url
  decoration  = false

  database_backup = true
  volume_backup   = true
  dokploy_backup  = true
}
