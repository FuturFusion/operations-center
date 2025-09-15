package terraform

import "slices"

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
