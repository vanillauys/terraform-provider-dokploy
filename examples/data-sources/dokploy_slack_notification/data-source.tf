data "dokploy_slack_notification" "example" {
  id = "your-notification-id"
}

data "dokploy_slack_notification" "ops" {
  name = "ops"
}
