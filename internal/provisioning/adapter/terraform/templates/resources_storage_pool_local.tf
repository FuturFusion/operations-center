resource "incus_storage_volume" "backups_per_node" {
  for_each = local.members

  name         = "backups"
  description  = "Volume holding system backups"
  target       = each.key
  pool         = incus_storage_pool.local.name
  type         = "custom"
  content_type = "filesystem"
}

resource "incus_storage_volume" "images_per_node" {
  for_each = local.members

  name         = "images"
  description  = "Volume holding system images"
  target       = each.key
  pool         = incus_storage_pool.local.name
  type         = "custom"
  content_type = "filesystem"
}
