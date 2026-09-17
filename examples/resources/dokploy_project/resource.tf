resource "dokploy_project" "example" {
  name        = "my-project"
  description = "Managed by Terraform"

  # Variables that each service in the project can reference as
  # ${{project.KEY}}. Use Terraform sensitive variables for secret values.
  env = <<-EOT
    REGION=eu-west-1
  EOT
}

output "production_environment_id" {
  value = dokploy_project.example.production_environment_id
}
