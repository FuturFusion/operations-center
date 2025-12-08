package api

import (
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/inventory"
)

func networkWithSyncFilter(network inventory.Network) bool {
	return !network.Object.Managed
}

func networkForwardWithParentFilter(network incusapi.Network) bool {
	switch network.Type {
	case "macvlan", "physical":
		return true
	}

	return !network.Managed
}

func networkLoadBalancerWithParentFilter(network incusapi.Network) bool {
	if !network.Managed {
		return true
	}

	switch network.Type {
	case "macvlan", "physical", "bridge":
		return true
	}

	return false
}

func networkPeerWithParentFilter(network incusapi.Network) bool {
	if !network.Managed {
		return true
	}

	switch network.Type {
	case "macvlan", "physical", "bridge":
		return true
	}

	return false
}

func storageBucketWithParentFilter(storagePool incusapi.StoragePool) bool {
	switch storagePool.Driver {
	case "btrfs", "cephobject", "dir", "zfs":
		// Only these drivers actually allow storage buckets.
		// Therefore these must not be filtered.
		return false
	default:
		return true
	}
}
