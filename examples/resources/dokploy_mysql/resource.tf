resource "dokploy_mysql" "example" {
  name              = "app-db"
  environment_id    = dokploy_project.example.production_environment_id
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password # use a sensitive variable
  docker_image      = "mysql:8"

  env = <<-EOT
    TZ=UTC
  EOT
}
