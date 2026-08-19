resource "dokploy_libsql" "example" {
  name              = "edge-db"
  environment_id    = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
  database_user     = "libsql"
  database_password = var.db_password # use a sensitive variable
  docker_image      = "ghcr.io/tursodatabase/libsql-server:v0.24.32"

  external_port       = 8080
  external_admin_port = 8081
  external_grpc_port  = 8082
}
