resource "dokploy_postgres" "example" {
  name              = "app-db"
  environment_id    = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password # use a sensitive variable
  docker_image      = "postgres:16-alpine"

  env = <<-EOT
    TZ=UTC
  EOT
}
