data "dokploy_application" "example" {
  id = "your-application-id"
}

# By name, within an environment.
data "dokploy_application" "by_name" {
  environment_id = data.dokploy_environment.production.id
  name           = "frontend"
}
