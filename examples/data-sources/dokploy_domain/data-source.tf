data "dokploy_domain" "example" {
  id = "your-domain-id"
}

# By host, limited to one compose service. Hosts are not unique in Dokploy,
# so a filter keeps the lookup unambiguous.
data "dokploy_domain" "mail" {
  host       = "mail.example.com"
  compose_id = data.dokploy_compose.stalwart.id
}

# By host alone. The lookup reads the domains of every service in the
# organization and fails if more than one carries the host.
data "dokploy_domain" "www" {
  host = "www.example.com"
}
