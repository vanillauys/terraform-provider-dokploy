data "dokploy_schedule" "example" {
  id = "your-schedule-id"
}

data "dokploy_schedule" "cleanup" {
  service_id    = data.dokploy_application.web.id
  schedule_type = "application"
  name          = "cleanup"
}
