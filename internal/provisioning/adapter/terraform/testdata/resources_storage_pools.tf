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
}

resource "incus_storage_pool" "shared" {
  name        = "shared"
  driver      = "lvmcluster"
  description = "Shared storage pool (lvmcluster)"

  config = {
  }

  depends_on = [incus_storage_pool.shared_per_node]
}
