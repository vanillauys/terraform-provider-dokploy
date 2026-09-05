# Send server alerts to a Telegram group topic through a bot.
resource "dokploy_telegram_notification" "alerts" {
  name              = "alerts"
  chat_id           = "-1001234567890"
  message_thread_id = "42"

  # Write-only: Terraform 1.11 or later. Change the version to rotate the token.
  bot_token_wo         = var.telegram_bot_token
  bot_token_wo_version = 1

  server_threshold = true
  dokploy_restart  = true
}
