# Vault providers import by their own id.
#
# Dokploy masks every secret field as "********" on every read (gate R,
# internal/client/doc.go, wave 6c), so no config block can be recovered by
# import - it is left null in the imported state. Re-supply the block
# matching the provider's actual type (hashicorp, infisical, aws, doppler,
# azure, or scaleway) in configuration; the first `terraform apply` after
# import is a full-body update, not an empty plan.
terraform import dokploy_vault_provider.secrets v1a2b3c4d5e6f7g8h9i0j
