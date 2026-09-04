resource "dokploy_project" "example" {
  name = "example"
}

# Dokploy creates a "production" environment with each project. This adds a
# second environment beside it.
resource "dokploy_environment" "staging" {
  project_id  = dokploy_project.example.id
  name        = "staging"
  description = "Pre-production environment"

  # Each service in this environment shares these variables.
  env = <<-EOT
    LOG_LEVEL=debug
    FEATURE_FLAGS=beta
  EOT
}
