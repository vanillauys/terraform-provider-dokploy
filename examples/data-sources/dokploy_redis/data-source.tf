data "dokploy_redis" "example" {
  id = "your-redis-id"
}

# By name, within an environment.
data "dokploy_redis" "by_name" {
  environment_id = data.dokploy_environment.production.id
  name           = "cache"
}
