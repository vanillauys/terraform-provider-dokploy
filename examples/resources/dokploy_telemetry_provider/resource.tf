# Grafana Loki for the container logs. Loki accepts logs only.
resource "dokploy_telemetry_provider" "loki" {
  name          = "loki"
  provider_type = "loki"
  signals       = ["logs"]
  endpoint      = "https://logs.example.com/loki/api/v1/push"

  # Write-only: Terraform 1.11 or later. Change the version to start an
  # update when only the token changed.
  api_key_wo         = var.loki_token
  api_key_wo_version = 1

  extra_config = {
    tenantId = "dokploy"
  }
}

# Datadog for the logs and the host metrics.
resource "dokploy_telemetry_provider" "datadog" {
  name          = "datadog"
  provider_type = "datadog"
  signals       = ["logs", "metrics"]
  api_key_wo    = var.datadog_api_key

  extra_config = {
    site = "datadoghq.eu"
  }
}

# Start the Vector agent on the Dokploy host with both providers.
resource "dokploy_web_server_settings" "this" {
  telemetry_provider_ids = [
    dokploy_telemetry_provider.loki.id,
    dokploy_telemetry_provider.datadog.id,
  ]
}
