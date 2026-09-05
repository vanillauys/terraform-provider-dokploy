# Post to a Microsoft Teams channel through an incoming webhook.
resource "dokploy_teams_notification" "deploys" {
  name = "deploys"

  webhook_url_wo         = var.teams_webhook_url
  webhook_url_wo_version = 1

  app_deploy      = true
  app_build_error = true
}
