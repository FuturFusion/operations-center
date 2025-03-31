package api

type InventoryAggregate struct {
	Cluster              string                `json:"cluster" yaml:"cluster"`
	Servers              []string              `json:"servers" yaml:"servers"`
	Images               []Image               `json:"images" yaml:"images"`
	Instances            []Instance            `json:"instances" yaml:"instances"`
	Networks             []Network             `json:"networks" yaml:"networks"`
	NetworkACLs          []NetworkACL          `json:"network_acls" yaml:"network_acls"`
	NetworkForwards      []NetworkForward      `json:"network_forwards" yaml:"network_forwards"`
	NetworkIntegrations  []NetworkIntegration  `json:"network_integrations" yaml:"network_integrations"`
	NetworkLoadBalancers []NetworkLoadBalancer `json:"network_load_balancers" yaml:"network_load_balancers"`
	NetworkPeers         []NetworkPeer         `json:"network_peers" yaml:"network_peers"`
	NetworkZones         []NetworkZone         `json:"network_zones" yaml:"network_zones"`
	Profiles             []Profile             `json:"profiles" yaml:"profiles"`
	Projects             []Project             `json:"projects" yaml:"projects"`
	StorageBuckets       []StorageBucket       `json:"storage_buckets" yaml:"storage_buckets"`
	StoragePools         []StoragePool         `json:"storage_pools" yaml:"storage_pools"`
	StorageVolumes       []StorageVolume       `json:"storage_volumes" yaml:"storage_volumes"`
}
