resource "dokploy_redis" "example" {
  name              = "app-cache"
  environment_id    = dokploy_project.example.environments[0].id
  database_password = var.db_password # use a sensitive variable
  docker_image      = "redis:8"

  env = <<-EOT
    TZ=UTC
  EOT
}
