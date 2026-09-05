# Emergency alerts through Pushover: priority 2 repeats every minute for an
# hour until someone acknowledges the message.
resource "dokploy_pushover_notification" "oncall" {
  name     = "on-call"
  priority = 2
  retry    = 60
  expire   = 3600

  user_key_wo          = var.pushover_user_key
  user_key_wo_version  = 1
  api_token_wo         = var.pushover_api_token
  api_token_wo_version = 1

  app_build_error  = true
  server_threshold = true
}
