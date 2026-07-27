resource "dokploy_mariadb" "example" {
  name              = "app-db"
  environment_id    = dokploy_project.example.environments[0].id
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password # use a sensitive variable
  docker_image      = "mariadb:11.4"

  env = <<-EOT
    TZ=UTC
  EOT
}
