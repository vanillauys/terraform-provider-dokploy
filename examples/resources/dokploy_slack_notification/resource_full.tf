# Test the webhook before the provider writes the record. The test sends a
# real message to the Slack channel. If the test fails, the apply fails and the
# provider writes nothing.
resource "dokploy_slack_notification" "checked" {
  name    = "deploys"
  channel = "#deploys"

  webhook_url_wo         = var.slack_webhook_url
  webhook_url_wo_version = 1

  app_deploy      = true
  app_build_error = true

  verify_connection = true
}
