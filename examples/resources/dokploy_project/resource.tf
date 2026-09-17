resource "dokploy_project" "example" {
  name        = "my-project"
  description = "Managed by Terraform"

  # Variables that each service in the project can reference as
  # ${{project.KEY}}. Use Terraform sensitive variables for secret values.
  env = <<-EOT
    REGION=eu-west-1
  EOT

  # Labels from dokploy_tag records. Omit the attribute for no tags.
  tag_ids = [dokploy_tag.production.id]
}

resource "dokploy_tag" "production" {
  name  = "production"
  color = "#0a8a74"
}

output "production_environment_id" {
  value = dokploy_project.example.production_environment_id
}
