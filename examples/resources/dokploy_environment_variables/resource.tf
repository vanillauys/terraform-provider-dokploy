# The variables of an application, as a map. The resource owns the whole
# env text of its target, so the target must not manage `env` itself.
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = dokploy_project.app.production_environment_id

  docker = {
    image = "ghcr.io/my-org/api:1.4.2"
  }

  # The application refreshes `env` from the server; without this block it
  # would plan to clear what the map wrote.
  lifecycle {
    ignore_changes = [env]
  }
}

resource "dokploy_environment_variables" "api" {
  application_id = dokploy_application.api.id

  variables = {
    PORT         = "8080"
    LOG_LEVEL    = "info"
    DATABASE_URL = "postgres://app:${var.db_password}@${dokploy_postgres.db.app_name}:5432/app"
  }
}

# Shared variables of an environment, which every service in it receives.
resource "dokploy_environment_variables" "staging" {
  environment_id = dokploy_environment.staging.id

  variables = {
    REGION        = "eu-west-1"
    FEATURE_FLAGS = "beta"
  }
}
