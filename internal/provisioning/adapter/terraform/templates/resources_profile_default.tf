resource "incus_profile" "default" {
  name = "default"
  // FIXME: should not be necessary, see https://github.com/lxc/terraform-provider-incus/pull/294
  project = "default"

  device {
    name = "root"
    type = "disk"
    properties = {
      path = "/"
      pool = incus_storage_pool.local.name
    }
  }

  device {
    name = "eth0"
    type = "nic"
    properties = {
      "network" = incus_network.incusbr0.name
    }
  }
}
