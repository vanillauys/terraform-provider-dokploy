resource "dokploy_project" "example" {
  name        = "my-project"
  description = "Managed by Terraform"
}

output "production_environment_id" {
  value = [for e in dokploy_project.example.environments : e.id if e.name == "production"][0]
}
