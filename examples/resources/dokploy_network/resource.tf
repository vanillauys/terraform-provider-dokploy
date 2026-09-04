resource "dokploy_network" "backend" {
  name = "backend-net"

  # Networks are immutable: a change to any attribute replaces the network.
  # attachable = true
  # mtu        = 1400
  # ipam = {
  #   config = [{ subnet = "172.28.0.0/16" }]
  # }
}

# Attach a service to it. The attachment applies on the next deploy of the service:
# resource "dokploy_application" "api" {
#   # ...
#   network_ids = [dokploy_network.backend.id]
# }
