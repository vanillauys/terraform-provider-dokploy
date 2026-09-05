# Post to a Mattermost channel through an incoming webhook.
resource "dokploy_mattermost_notification" "deploys" {
  name     = "deploys"
  channel  = "deploys"
  username = "dokploy"

  webhook_url_wo         = var.mattermost_webhook_url
  webhook_url_wo_version = 1

  app_deploy = true
}
