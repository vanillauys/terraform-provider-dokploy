data "dokploy_mongo" "example" {
  id = "your-mongo-id"
}

# By name, within an environment.
data "dokploy_mongo" "by_name" {
  environment_id = data.dokploy_environment.production.id
  name           = "db"
}
