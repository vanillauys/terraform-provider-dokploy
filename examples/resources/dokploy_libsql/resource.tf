resource "dokploy_libsql" "example" {
  name              = "edge-db"
  environment_id    = dokploy_project.example.production_environment_id
  database_user     = "libsql"
  database_password = var.db_password # use a sensitive variable
  docker_image      = "ghcr.io/tursodatabase/libsql-server:v0.24.32"

  external_port       = 8080
  external_admin_port = 8081
  external_grpc_port  = 8082
}

# A replica follows a primary in the same environment. On Dokploy v0.30.5 the
# role and the URL alone do not make the container replicate; the command
# override does. Do not add --grpc-listen-addr to a replica command.
resource "dokploy_libsql" "replica" {
  name              = "edge-db-replica"
  environment_id    = dokploy_project.example.production_environment_id
  database_user     = "libsql"
  database_password = var.db_password
  sqld_node         = "replica"
  sqld_primary_url  = "http://${dokploy_libsql.example.app_name}:5001"
  command           = "sqld --db-path iku.db --http-listen-addr 0.0.0.0:8080 --admin-listen-addr 0.0.0.0:5000 --primary-grpc-url http://${dokploy_libsql.example.app_name}:5001"
}
