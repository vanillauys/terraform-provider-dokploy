data "dokploy_vault_provider" "example" {
  id = "your-vault-provider-id"
}

data "dokploy_vault_provider" "prod" {
  name = "prod"
}

# The name is the first part of the vault reference in an env value.
resource "dokploy_application" "api" {
  name           = "api"
  environment_id = data.dokploy_environment.production.id
  env            = "DATABASE_URL=$${{vault.${data.dokploy_vault_provider.prod.name}.DATABASE_URL}}"

  docker = { image = "ghcr.io/example/api:latest" }
}
