data "dokploy_tag" "example" {
  id = "your-tag-id"
}

data "dokploy_tag" "production" {
  name = "production"
}

# A project that carries a tag created outside Terraform.
resource "dokploy_project" "site" {
  name    = "site"
  tag_ids = [data.dokploy_tag.production.id]
}
