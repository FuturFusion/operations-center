package api

import "time"

// Cluster defines a cluster of servers running Hypervisor OS.
//
// swagger:model
type Cluster struct {
	// A human-friendly name for this cluster.
	// Example: MyCluster
	Name string `json:"name" yaml:"name"`

	// URL, hostname or IP address of the cluster endpoint.
	// Example: https://incus.local:6443
	ConnectionURL string `json:"connection_url" yaml:"connection_url"`

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}

// ClusterPost represents the fields available for a new cluster of servers running Hypervisor OS.
//
// swagger:model
type ClusterPost struct {
	Cluster `yaml:",inline"`

	// Names of the servers beloning to the cluster.
	// Example: [ "server1", "server2" ]
	ServerNames []string `json:"server_names" yaml:"server_names"`
}
