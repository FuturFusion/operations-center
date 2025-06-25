package api

import (
	"database/sql/driver"
	"fmt"
	"time"
)

type ClusterStatus string

const (
	ClusterStatusUnknown ClusterStatus = "unknown"
	ClusterStatusPending ClusterStatus = "pending"
	ClusterStatusReady   ClusterStatus = "ready"
)

var ClusterStatuses = map[ClusterStatus]struct{}{
	ClusterStatusUnknown: {},
	ClusterStatusPending: {},
	ClusterStatusReady:   {},
}

func (s ClusterStatus) String() string {
	return string(s)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (s ClusterStatus) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *ClusterStatus) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*s = ClusterStatusUnknown
		return nil
	}

	_, ok := ClusterStatuses[ClusterStatus(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid server status", string(text))
	}

	*s = ClusterStatus(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (s ClusterStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface.
func (s *ClusterStatus) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid server status")
	}

	switch v := value.(type) {
	case string:
		return s.UnmarshalText([]byte(v))
	case []byte:
		return s.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for server status", value)
	}
}

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

	// Status contains the status the cluster is currently in from the point of view of Operations Center.
	// Possible values for status are: pending, ready
	// Example: pending
	Status ClusterStatus

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
