package api

import (
	incusapi "github.com/lxc/incus/v6/shared/api"
)

type InventoryAggregate struct {
	Cluster              string                                                             `json:"cluster" yaml:"cluster"`
	Servers              []string                                                           `json:"servers" yaml:"servers"`
	Images               map[string]map[string]incusapi.Image                               `json:"images" yaml:"images"`                                 // by project_name, name
	Instances            map[string]map[string]map[string]incusapi.InstanceFull             `json:"instances" yaml:"instances"`                           // by project_name, name, server_name
	Networks             map[string]map[string]incusapi.Network                             `json:"networks" yaml:"networks"`                             // by project_name, name
	NetworkACLs          map[string]map[string]incusapi.NetworkACL                          `json:"network_acls" yaml:"network_acls"`                     // by project_name, name
	NetworkForwards      map[string]map[string]incusapi.NetworkForward                      `json:"network_forwards" yaml:"network_forwards"`             // by network_name, name
	NetworkIntegrations  map[string]incusapi.NetworkIntegration                             `json:"network_integrations" yaml:"network_integrations"`     // by name
	NetworkLoadBalancers map[string]map[string]incusapi.NetworkLoadBalancer                 `json:"network_load_balancers" yaml:"network_load_balancers"` // by network_name, name
	NetworkPeers         map[string]map[string]incusapi.NetworkPeer                         `json:"network_peers" yaml:"network_peers"`                   // by network_name, name
	NetworkZones         map[string]map[string]incusapi.NetworkZone                         `json:"network_zones" yaml:"network_zones"`                   // by project_name, name
	Profiles             map[string]map[string]incusapi.Profile                             `json:"profiles" yaml:"profiles"`                             // by project_name, name
	Projects             map[string]incusapi.Project                                        `json:"projects" yaml:"projects"`                             // by name
	StorageBuckets       map[string]map[string]map[string]map[string]incusapi.StorageBucket `json:"storage_buckets" yaml:"storage_buckets"`               // by project_name, storage_pool, name, server_name
	StoragePools         map[string]incusapi.StoragePool                                    `json:"storage_pools" yaml:"storage_pools"`                   // by name
	StorageVolumes       map[string]map[string]map[string]map[string]incusapi.StorageVolume `json:"storage_volumes" yaml:"storage_volumes"`               // by project_name, storage_pool, type/name, server_name
}
