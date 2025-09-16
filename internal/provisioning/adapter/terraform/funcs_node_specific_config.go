package terraform

import "slices"

// nodeSpecificConfig lists all config keys, which are node-specific.
// Extracted from https://github.com/lxc/incus/blob/a2adb4f79580f5b25fe6cc55ef140e321a10682a/internal/server/node/config.go#L163
var nodeSpecificConfig = []string{
	"core.https_address",
	"cluster.https_address",
	"core.bgp_address",
	"core.bgp_routerid",
	"core.debug_address",
	"core.dns_address",
	"core.metrics_address",
	"core.storage_buckets_address",
	"core.syslog_socket",
	"network.ovs.connection",
	"storage.backups_volume",
	"storage.images_volume",
	"storage.linstor.satellite.name",
}

func isNodeSpecificConfig(name string) bool {
	return slices.Contains(nodeSpecificConfig, name)
}

// nodeSpecificStorageConfig lists all storage pool config keys which are node-specific.
// This is copied from https://github.com/lxc/incus/blob/a2adb4f79580f5b25fe6cc55ef140e321a10682a/internal/server/db/storage_pools.go#L878-L888
var nodeSpecificStorageConfig = []string{
	"size",
	"source",
	"source.wipe",
	"volatile.initial_source",
	"zfs.pool_name",
	"lvm.thinpool_name",
	"lvm.vg_name",
	"lvm.vg.force_reuse",
}

func isNodeSpecificStorageConfig(name string) bool {
	return slices.Contains(nodeSpecificStorageConfig, name)
}

// nodeSpecificNetworkConfig lists all static network config keys which are node-specific.
// This is copied from https://github.com/lxc/incus/blob/e4b571a470ca8c9d9e21245664eed1149139d2bb/internal/server/db/networks.go#L889-L895
var nodeSpecificNetworkConfig = []string{
	"bgp.ipv4.nexthop",
	"bgp.ipv6.nexthop",
	"bridge.external_interfaces",
	"parent",
}

func isNodeSpecificNetworkConfig(name string) bool {
	return slices.Contains(nodeSpecificNetworkConfig, name)
}
