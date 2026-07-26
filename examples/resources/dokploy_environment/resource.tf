resource "dokploy_project" "example" {
  name = "example"
}

# Dokploy creates a "production" environment with every project; this adds a
# second one alongside it.
resource "dokploy_environment" "staging" {
  project_id  = dokploy_project.example.id
  name        = "staging"
  description = "Pre-production environment"

  # Shared by every service in this environment.
  env = <<-EOT
    LOG_LEVEL=debug
    FEATURE_FLAGS=beta
  EOT
}
