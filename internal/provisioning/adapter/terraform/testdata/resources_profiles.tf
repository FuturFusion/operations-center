
resource "incus_profile" "default" {
  name        = "default"
  description = ""

  config = {
  }

  device {
    name = "eth0"
    type = "nic"
    properties = {
      "network" = "incusbr0"
    }
  }

  device {
    name = "root"
    type = "disk"
    properties = {
      "path" = "/"
      "pool" = "local"
    }
  }

  depends_on = [
    // TODO: Should go after all networks
    // TODO: Should go after all projects

    incus_network.incusbr0,
    incus_network.meshbr0,
    incus_project.internal,
  ]
}


resource "incus_profile" "internal_default" {
  name        = "default"
  description = ""

  project = "internal"

  config = {
  }

  device {
    name = "eth0"
    type = "nic"
    properties = {
      "network" = "meshbr0"
    }
  }

  device {
    name = "root"
    type = "disk"
    properties = {
      "path" = "/"
      "pool" = "local"
    }
  }

  depends_on = [
    // TODO: Should go after all networks
    // TODO: Should go after all projects

    incus_network.incusbr0,
    incus_network.meshbr0,
    incus_project.internal,
  ]
}

resource "null_resource" "post_profiles" {
  depends_on = [
    incus_profile.default,
    incus_profile.internal_default,
  ]
}
