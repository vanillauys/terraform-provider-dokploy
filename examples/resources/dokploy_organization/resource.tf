# Keep the organization record under Terraform: its name, logo, and the role
# that a new member gets. The provider itself always works inside the API
# key's active organization.
resource "dokploy_organization" "main" {
  name         = "Acme"
  logo         = "https://acme.example.com/logo.png"
  default_role = "member"
}
