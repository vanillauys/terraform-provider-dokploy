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

# Phase.dev (Dokploy v0.30.5 and later).
resource "dokploy_vault_provider" "phase" {
  name = "phase"

  phase = {
    token_wo         = var.phase_token
    token_wo_version = 1
    app_id           = "app_0123456789"
    env              = "production"
    # path    = "/"                     # Server default.
    # api_url = "https://api.phase.dev" # Server default. Set it for a self-hosted Phase.
  }

  assignments = []
}

# AWS Systems Manager Parameter Store (Dokploy v0.30.6 and later).
resource "dokploy_vault_provider" "ssm" {
  name = "ssm"

  aws_parameter_store = {
    region            = "eu-west-1"
    access_key_id     = var.aws_access_key_id
    secret_access_key = var.aws_secret_access_key
    parameter_path    = "/wihan-dev/" # Optional. It must start with `/`.
  }

  assignments = []
}
