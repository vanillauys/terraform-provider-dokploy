resource "dokploy_project" "example" {
  name        = "my-project"
  description = "Managed by Terraform"
}

output "production_environment_id" {
  value = dokploy_project.example.production_environment_id
}
