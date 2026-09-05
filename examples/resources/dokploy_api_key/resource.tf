# A key for a CI pipeline. Dokploy returns the key once; Terraform keeps it
# in the state as a sensitive value. Every attribute replaces the key on
# change, so a rename is a rotation.
resource "dokploy_api_key" "ci" {
  name   = "github-actions"
  prefix = "ci"
}

# Hand the key to the pipeline through an output or a secret store.
output "ci_api_key" {
  value     = dokploy_api_key.ci.key
  sensitive = true
}

# A key that expires after 30 days and allows 600 requests per minute.
resource "dokploy_api_key" "review" {
  name                   = "review-bot"
  expires_in             = 30 * 24 * 60 * 60
  rate_limit_enabled     = true
  rate_limit_max         = 600
  rate_limit_time_window = 60 * 1000
}
