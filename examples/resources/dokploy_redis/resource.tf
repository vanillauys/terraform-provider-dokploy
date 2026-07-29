resource "dokploy_redis" "example" {
  name              = "app-cache"
  environment_id    = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
  database_password = var.db_password # use a sensitive variable
  docker_image      = "redis:8"

  env = <<-EOT
    TZ=UTC
  EOT
}
