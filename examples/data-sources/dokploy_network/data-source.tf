# Look up a network created (or imported) in the Dokploy UI.
data "dokploy_network" "shared" {
  name = "backend-net"
}
