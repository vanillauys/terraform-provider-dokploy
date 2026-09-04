# Look up a network that the Dokploy UI created or imported.
data "dokploy_network" "shared" {
  name = "backend-net"
}
