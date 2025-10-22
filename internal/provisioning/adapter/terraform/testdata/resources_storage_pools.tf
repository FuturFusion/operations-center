resource "incus_storage_pool" "shared_per_node" {
  for_each = local.members

  name   = "shared"
  driver = "lvmcluster"
  target = each.key

  config = {
    "lvm.vg_name" = "vg0"
    "source"      = "/dev/sda"
  }

  lifecycle {
    ignore_changes = [
      # Incus changes the source behind the scenes so we ignore these changes.
      config["source"],
    ]
  }

  depends_on = []
}

resource "incus_storage_pool" "shared" {
  name        = "shared"
  driver      = "lvmcluster"
  description = "Shared storage pool (lvmcluster)"

  config = {
  }

  depends_on = [
    incus_storage_pool.shared_per_node
  ]
}

resource "incus_storage_pool" "local_per_node" {
  for_each = local.members

  name   = "local"
  driver = "zfs"
  target = each.key

  config = {
    "source" = "local/incus"
  }

  lifecycle {
    ignore_changes = [
      # Incus changes the source behind the scenes so we ignore these changes.
      config["source"],
    ]
  }

  depends_on = []
}

resource "incus_storage_pool" "local" {
  name        = "local"
  driver      = "zfs"
  description = "Local storage pool (on system drive)"

  config = {
  }

  depends_on = [
    incus_storage_pool.local_per_node
  ]
}

resource "null_resource" "post_storage_pools" {
  depends_on = [
    incus_storage_pool.shared,
    incus_storage_pool.local,
  ]
}
