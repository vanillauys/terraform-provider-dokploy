resource "dokploy_tag" "production" {
  name  = "production"
  color = "#0a8a74"
}

# A tag without a colour.
resource "dokploy_tag" "internal" {
  name = "internal"
}

# A project carries a set of tags; the provider replaces the whole set on
# each change.
resource "dokploy_project" "site" {
  name    = "site"
  tag_ids = [dokploy_tag.production.id, dokploy_tag.internal.id]
}
