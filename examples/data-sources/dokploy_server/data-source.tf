# Look up a remote server that already exists in Dokploy, by name.
data "dokploy_server" "worker" {
  name = "worker-1"
}

# Or by id, when the name is ambiguous or you already hold the id.
data "dokploy_server" "by_id" {
  id = "cnWbR6INlpglaAu8MML5X"
}

# The usual purpose of a lookup: run a service on a server that the Dokploy
# UI already manages.
resource "dokploy_redis" "cache" {
  name           = "cache"
  environment_id = dokploy_project.app.production_environment_id
  server_id      = data.dokploy_server.worker.id
}
