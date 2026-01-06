package terraform

import (
	"reflect"
	"slices"
)

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

// nodeSpecificNetworkConfig lists all static network config keys which are node-specific.
// This is copied from https://github.com/lxc/incus/blob/e4b571a470ca8c9d9e21245664eed1149139d2bb/internal/server/db/networks.go#L889-L895
var nodeSpecificNetworkConfig = []string{
	"bgp.ipv4.nexthop",
	"bgp.ipv6.nexthop",
	"bridge.external_interfaces",
	"parent",
}

type splitConfigs struct {
	Specific map[string]string
	Global   map[string]string
}

func splitConfig(m any, kind string) splitConfigs {
	v := reflect.ValueOf(m)

	if v.Kind() != reflect.Map {
		panic("config is not a map")
	}

	if v.Type().Key().Kind() != reflect.String {
		panic("config key is not string")
	}

	var lookup []string
	switch kind {
	case "node":
		lookup = nodeSpecificConfig

	case "storage":
		lookup = nodeSpecificStorageConfig

	case "network":
		lookup = nodeSpecificNetworkConfig

	default:
		panic("kind not supported, allowed values: node, storage, network")
	}

	specific := map[string]string{}
	global := map[string]string{}

	iter := reflect.ValueOf(m).MapRange()
	for iter.Next() {
		k := iter.Key()
		v := iter.Value()

		if slices.Contains(lookup, k.String()) {
			specific[k.String()] = v.String()
		} else {
			global[k.String()] = v.String()
		}
	}

	return splitConfigs{
		Specific: specific,
		Global:   global,
	}
}
