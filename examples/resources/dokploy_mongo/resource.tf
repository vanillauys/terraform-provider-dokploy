resource "dokploy_mongo" "example" {
  name              = "app-db"
  environment_id    = dokploy_project.example.production_environment_id
  database_user     = "app"
  database_password = var.db_password # use a sensitive variable
  docker_image      = "mongo:7"

  env = <<-EOT
    TZ=UTC
  EOT
}
