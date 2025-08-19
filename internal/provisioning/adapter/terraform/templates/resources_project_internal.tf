resource "incus_project" "internal" {
  name        = "internal"
  description = "Internal project to isolate fully managed resources."
}

resource "incus_profile" "internal-default" {
  name    = "default"
  project = incus_project.internal.name

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
      "network" = incus_network.meshbr0.name
    }
  }
}
