# Post deploy and build-failure messages to a Slack channel.
resource "dokploy_slack_notification" "deploys" {
  name    = "deploys"
  channel = "#deploys"

  # Write-only: Terraform 1.11 or later. Change the version to rotate the webhook.
  webhook_url_wo         = var.slack_webhook_url
  webhook_url_wo_version = 1

  app_deploy      = true
  app_build_error = true
}
