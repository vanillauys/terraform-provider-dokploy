# Look up an SSH key that already exists in Dokploy, by name.
data "dokploy_ssh_key" "deploy" {
  name = "deploy"
}

# Or by id, when the name is ambiguous or you already hold the id.
data "dokploy_ssh_key" "by_id" {
  id = "UvJz8naQ2Q1G26LblagBS"
}

# The usual purpose of a lookup: a key that exists once and that several
# servers or private git sources reference.
resource "dokploy_server" "worker" {
  name       = "worker-1"
  ip_address = "203.0.113.10"
  ssh_key_id = data.dokploy_ssh_key.deploy.id
}
