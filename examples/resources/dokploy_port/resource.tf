resource "dokploy_port" "metrics" {
  application_id = dokploy_application.web.id
  published_port = 9100
  target_port    = 9100
  protocol       = "tcp"
  publish_mode   = "host"
}
