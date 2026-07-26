data "dokploy_postgres" "example" {
  id = "your-postgres-id"
}

# By name, within an environment.
data "dokploy_postgres" "by_name" {
  environment_id = data.dokploy_environment.production.id
  name           = "db"
}
