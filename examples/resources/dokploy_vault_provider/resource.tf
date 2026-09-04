variable "vault_token" {
  type      = string
  sensitive = true
}

resource "dokploy_vault_provider" "secrets" {
  name = "prod-vault"

  hashicorp = {
    url   = "https://vault.example.com:8200"
    token = var.vault_token
    # namespace = "admin" # Vault Enterprise only. Omit it for open-source Vault or OpenBao.
    # mount     = "secret" # KV mount path. This is the server default.
  }

  assignments = [
    {
      project_id = dokploy_project.example.id
      # environment_ids = [] # Omit it, or leave it empty, to cover each
      #                        environment in the project.
    }
  ]

  # Tests the real vault through vaultProvider.testConnection before the
  # write, so a bad token or an unreachable server fails the apply instead
  # of a broken vault provider.
  verify_connection = true
}

# Reference a secret from this vault provider in the `env` of another resource.
# Dokploy resolves ${{vault.<name>.<key>}} at deploy time. The provider
# passes the string through unchanged and does not parse or validate it.
# The doubled `$$` escapes the Terraform `${...}` interpolation, so the
# literal `${{...}}` reaches Dokploy.
#
# resource "dokploy_application" "api" {
#   # ...
#   env = <<-EOT
#     DATABASE_PASSWORD=$${{vault.prod-vault.database_password}}
#   EOT
# }
