# Dokploy masks every secret field as "********" on every read. Import
# cannot recover the config block, so the imported state holds a null block.
# Write the block for the type of the provider in the configuration: hashicorp,
# infisical, aws, doppler, azure, or scaleway. The first apply after the
# import is a full update. It is not an empty plan.
import {
  to = dokploy_vault_provider.secrets
  id = "v1a2b3c4d5e6f7g8h9i0j"
}
