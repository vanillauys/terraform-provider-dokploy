# The active organization: the one that every resource of this provider
# lands in.
data "dokploy_organization" "current" {}

output "organization_id" {
  value = data.dokploy_organization.current.id
}

# Or look up another organization of the same owner, by name or by id.
data "dokploy_organization" "staging" {
  name = "Acme Staging"
}
