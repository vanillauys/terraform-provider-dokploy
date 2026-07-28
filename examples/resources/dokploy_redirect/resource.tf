resource "dokploy_redirect" "legacy_blog" {
  application_id = dokploy_application.web.id
  regex          = "^/blog/(.*)"
  replacement    = "/posts/$1"
  permanent      = true
}
