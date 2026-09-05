# Permissions of a member that was invited in the Dokploy UI. The lookup by
# email gives the user id.
data "dokploy_user" "contractor" {
  email = "contractor@example.com"
}

resource "dokploy_user_permissions" "contractor" {
  user_id = data.dokploy_user.contractor.id

  accessed_projects     = [dokploy_project.app.id]
  accessed_environments = [dokploy_environment.staging.id]

  can_create_services = true
  can_delete_services = false
  can_access_docker   = true
}
