# Post to a Lark (Feishu) group through a bot webhook.
resource "dokploy_lark_notification" "deploys" {
  name = "deploys"

  webhook_url_wo         = var.lark_webhook_url
  webhook_url_wo_version = 1

  app_deploy      = true
  app_build_error = true
}
