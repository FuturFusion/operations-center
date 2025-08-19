resource "incus_network" "incusbr0_per_node" {
  for_each = local.members

  name   = "incusbr0"
  target = each.key
  type   = "bridge"
}

resource "incus_network" "incusbr0" {
  name        = "incusbr0"
  description = "Local network bridge (NAT)"

  depends_on = [incus_network.incusbr0_per_node]
}

// Generate random values for the meshbr0 IPv6 subnet.
resource "random_integer" "meshbr0-subnet-segment-2" {
  min = 0
  max = 65535
}
resource "random_integer" "meshbr0-subnet-segment-3" {
  min = 0
  max = 65535
}
resource "random_integer" "meshbr0-subnet-segment-4" {
  min = 0
  max = 65535
}

resource "incus_network" "meshbr0_per_node" {
  for_each = local.members

  name   = "meshbr0"
  target = each.key
  type   = "bridge"

  config = {
    "tunnel.mesh.interface" = local.meshTunnelInterfaces[each.key]
  }
}

resource "incus_network" "meshbr0" {
  name        = "meshbr0"
  description = "Internal mesh network bridge"

  config = {
    "ipv4.address"         = "none"
    "ipv6.address"         = format("fd42:%x:%x:%x::/64", random_integer.meshbr0-subnet-segment-2.result, random_integer.meshbr0-subnet-segment-3.result, random_integer.meshbr0-subnet-segment-4.result)
    "tunnel.mesh.id"       = "1000"
    "tunnel.mesh.protocol" = "vxlan"
  }

  depends_on = [incus_network.meshbr0_per_node]
}
