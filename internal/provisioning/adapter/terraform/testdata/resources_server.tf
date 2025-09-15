resource "incus_server" "this_per_node" {
  for_each = local.members

  config = {
    "storage.backups_volume" = "local/backups"
    "storage.images_volume"  = "shared"
  }

  depends_on = [
    incus_storage_volume.backups_per_node,
    incus_storage_volume.images_per_node,
    incus_storage_pool.shared,
  ]

  target = each.key
}

resource "incus_server" "this" {
  config = {
    "user.ui.sso_only" = "true"
  }

  depends_on = [
    null_resource.post_projects,
    null_resource.post_networks,
    null_resource.post_storage_pools,
    null_resource.post_storage_volumes,
    incus_server.this_per_node,
  ]
}
