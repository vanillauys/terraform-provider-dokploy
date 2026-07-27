data "dokploy_mysql" "example" {
  id = "your-mysql-id"
}

# By name, within an environment.
data "dokploy_mysql" "by_name" {
  environment_id = data.dokploy_environment.production.id
  name           = "db"
}
