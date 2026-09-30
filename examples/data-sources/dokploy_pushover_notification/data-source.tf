data "dokploy_pushover_notification" "example" {
  id = "your-notification-id"
}

data "dokploy_pushover_notification" "ops" {
  name = "ops"
}
