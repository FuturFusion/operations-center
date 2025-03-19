package inventory

import (
	incusapi "github.com/lxc/incus/v6/shared/api"
)

type InventoryAggregate struct {
	Cluster              string
	Servers              []string
	Images               map[string]map[string]incusapi.Image                               // by project_name, name
	Instances            map[string]map[string]map[string]incusapi.InstanceFull             // by server_name, project_name, name
	Networks             map[string]map[string]incusapi.Network                             // by project_name, name
	NetworkACLs          map[string]map[string]incusapi.NetworkACL                          // by project_name, name
	NetworkForwards      map[string]map[string]incusapi.NetworkForward                      // by network_name, name
	NetworkIntegrations  map[string]incusapi.NetworkIntegration                             // by name
	NetworkLoadBalancers map[string]map[string]incusapi.NetworkLoadBalancer                 // by network_name, name
	NetworkPeers         map[string]map[string]incusapi.NetworkPeer                         // by network_name, name
	NetworkZones         map[string]map[string]incusapi.NetworkZone                         // by project_name, name
	Profiles             map[string]map[string]incusapi.Profile                             // by project_name, name
	Projects             map[string]incusapi.Project                                        // by name
	StorageBuckets       map[string]map[string]map[string]map[string]incusapi.StorageBucket // by server_name, project_name, storage_pool, name
	StoragePools         map[string]incusapi.StoragePool                                    // by name
	StorageVolumes       map[string]map[string]map[string]map[string]incusapi.StorageVolume // by server_name, project_name, storage_pool, type/name
}

type InventoryAggregates []InventoryAggregate

type InventoryAggregateColumns struct {
	Servers              bool
	Images               bool
	Instances            bool
	Networks             bool
	NetworkACLs          bool
	NetworkForwards      bool
	NetworkIntegrations  bool
	NetworkLoadBalancers bool
	NetworkPeers         bool
	NetworkZones         bool
	Profiles             bool
	Projects             bool
	StorageBuckets       bool
	StoragePools         bool
	StorageVolumes       bool
}

type InventoryAggregateFilter struct {
	Cluster    *string
	Expression *string
}
