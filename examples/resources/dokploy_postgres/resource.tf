resource "dokploy_postgres" "example" {
  name              = "app-db"
  environment_id    = dokploy_project.example.production_environment_id
  database_name     = "app"
  database_user     = "app"
  database_password = var.db_password # use a sensitive variable
  docker_image      = "postgres:16-alpine"

  env = <<-EOT
    TZ=UTC
  EOT

  # Resource limits as whole numbers: memory in bytes, CPU in nano-CPUs.
  # Dokploy reads them with parseInt, so "1g" or "0.5" do not work. A change
  # starts a redeploy.
  memory_limit       = "1073741824" # 1 GiB
  memory_reservation = "536870912"  # 512 MiB
  # cpu_limit       = "1000000000"  # one CPU
  # cpu_reservation = "500000000"   # half a CPU
  # replicas        = 1
  # command         = "docker-entrypoint.sh"
  # args            = ["postgres", "-c", "max_connections=200"]
}
