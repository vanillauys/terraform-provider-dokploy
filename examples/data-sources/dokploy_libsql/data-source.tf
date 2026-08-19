data "dokploy_libsql" "example" {
  id = "your-libsql-id"
}

# By name, within an environment.
data "dokploy_libsql" "by_name" {
  environment_id = data.dokploy_environment.production.id
  name           = "db"
}
