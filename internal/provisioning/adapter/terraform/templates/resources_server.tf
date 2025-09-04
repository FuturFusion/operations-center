resource "incus_server" "local" {
  for_each = local.members

  config = {
    "storage.backups_volume" = "${incus_storage_pool.local.name}/backups"
    "storage.images_volume"  = "${incus_storage_pool.local.name}/images"
  }

  depends_on = [
    incus_storage_volume.backups_per_node,
    incus_storage_volume.images_per_node,
  ]

  target = each.key
}
