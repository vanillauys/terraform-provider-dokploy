# Look up a member of the active organization by email, for example a
# person who was invited in the Dokploy UI.
data "dokploy_user" "dev" {
  email = "dev@example.com"
}

# Or by user id.
data "dokploy_user" "by_id" {
  id = "rM64isnUKMgqgOnwm7zE3"
}

# The usual purpose: manage that person's permissions.
resource "dokploy_user_permissions" "dev" {
  user_id             = data.dokploy_user.dev.id
  accessed_projects   = [dokploy_project.app.id]
  can_create_services = true
}
