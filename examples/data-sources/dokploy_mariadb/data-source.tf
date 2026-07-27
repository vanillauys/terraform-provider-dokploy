data "dokploy_mariadb" "example" {
  id = "your-mariadb-id"
}

# By name, within an environment.
data "dokploy_mariadb" "by_name" {
  environment_id = data.dokploy_environment.production.id
  name           = "db"
}
