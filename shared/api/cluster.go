package api

import "time"

// Cluster defines a cluster of servers running Hypervisor OS.
//
// swagger:model
type Cluster struct {
	// An opaque integer identifier for the cluster.
	// Example: 1
	ID int `json:"id" yaml:"id"`

	// A human-friendly name for this cluster.
	// Example: MyCluster
	Name string `json:"name" yaml:"name"`

	// URL, hostname or IP address of the cluster endpoint.
	// Example: https://incus.local:6443
	ConnectionURL string `json:"connection_url" yaml:"connection_url"`

	// ServerHostnames ...
	ServerHostnames []string `json:"server_hostnames" yaml:"server_hostnames"` // FIXME: it is unclear, if this is needed. If we don't have this, how can we know the expected size of a cluster and prevent it from growing / shrinking?

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}
