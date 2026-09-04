# Nightly cleanup inside the container of an application.
resource "dokploy_schedule" "prune" {
  name            = "nightly-prune"
  schedule_type   = "application"
  service_id      = dokploy_application.web.id
  cron_expression = "0 3 * * *"
  command         = "node scripts/prune.js"
  timezone        = "Africa/Johannesburg"
}

# A job on the Dokploy host takes no service_id.
resource "dokploy_schedule" "host_df" {
  name            = "disk-report"
  schedule_type   = "dokploy-server"
  cron_expression = "0 8 * * 1"
  shell_type      = "sh"
  command         = "df -h /"
}
