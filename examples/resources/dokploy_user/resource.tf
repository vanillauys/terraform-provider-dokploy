# A team member with an initial password. The person signs in with the email
# and the password, then changes the password in Dokploy.
resource "dokploy_user" "dev" {
  email = "dev@example.com"
  role  = "member"

  # Write-only: Terraform 1.11 or later. A version change replaces the
  # account, because Dokploy cannot reset another user's password.
  password_wo         = var.dev_initial_password
  password_wo_version = 1
}

# An admin with a plain password attribute.
resource "dokploy_user" "ops" {
  email    = "ops@example.com"
  password = var.ops_initial_password
  role     = "admin"
}

# Give the member access to one project and the right to create services.
resource "dokploy_user_permissions" "dev" {
  user_id             = dokploy_user.dev.id
  accessed_projects   = [dokploy_project.app.id]
  can_create_services = true
}
