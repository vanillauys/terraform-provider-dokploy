variable "vault_token" {
  type      = string
  sensitive = true
}

resource "dokploy_vault_provider" "secrets" {
  name = "prod-vault"

  hashicorp = {
    url   = "https://vault.example.com:8200"
    token = var.vault_token
    # namespace = "admin" # Vault Enterprise only; omit for OSS Vault or OpenBao.
    # mount     = "secret" # KV mount path; this is the server's own default.
  }

  assignments = [
    {
      project_id = dokploy_project.example.id
      # environment_ids = [] # Omit (or leave empty) to make every
      #                        environment in the project eligible.
    }
  ]

  # Reaches the real vault through vaultProvider.testConnection before
  # writing anything, so a bad token or an unreachable server fails the
  # apply instead of silently creating a broken vault provider.
  verify_connection = true
}

# Reference a secret from this vault provider in another resource's `env`.
# Dokploy resolves ${{vault.<name>.<key>}} at deploy time; this provider
# passes the string through untouched - it does not parse or validate it.
# The doubled `$$` escapes Terraform's own `${...}` interpolation so the
# literal `${{...}}` reaches Dokploy.
#
# resource "dokploy_application" "api" {
#   # ...
#   env = <<-EOT
#     DATABASE_PASSWORD=$${{vault.prod-vault.database_password}}
#   EOT
# }
