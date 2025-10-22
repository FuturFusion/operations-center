resource "incus_cluster_group" "cluster_group1" {
  name        = "cluster_group1"
  description = "cluster group 1"

  config = {
    "key"       = "value",
    "other_key" = "other_value",
  }

  depends_on = []
}

resource "incus_cluster_group_member" "cluster_group1_server1" {
  cluster_group = incus_cluster_group.cluster_group1.name
  member        = server1
}

resource "incus_cluster_group_member" "cluster_group1_server2" {
  cluster_group = incus_cluster_group.cluster_group1.name
  member        = server2
}

resource "null_resource" "post_cluster_groups" {
  depends_on = [
    incus_cluster_group.cluster_group1,
    incus_cluster_group_member.cluster_group1_server1,
    incus_cluster_group_member.cluster_group1_server2,
  ]
}
