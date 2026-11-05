data "dokploy_telemetry_provider" "example" {
  id = "your-telemetry-provider-id"
}

data "dokploy_telemetry_provider" "loki" {
  name = "loki"
}
