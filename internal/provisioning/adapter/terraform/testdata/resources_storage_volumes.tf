resource "incus_storage_volume" "backups" {
  for_each = local.members

  name         = "backups"
  description  = "Volume holding system backups"
  target       = each.key
  pool         = "local"
  type         = "custom"
  content_type = "filesystem"

  config = {
  }

  depends_on = [
    null_resource.post_storage_pools,
    null_resource.post_projects,
  ]
}

resource "incus_storage_volume" "images" {
  for_each = local.members

  name         = "images"
  description  = "Volume holding system images"
  target       = each.key
  pool         = "local"
  type         = "custom"
  content_type = "filesystem"

  config = {
  }

  depends_on = [
    null_resource.post_storage_pools,
    null_resource.post_projects,
  ]
}

resource "null_resource" "post_storage_volumes" {
  depends_on = [
    incus_storage_volume.backups,
    incus_storage_volume.images,
  ]
}
