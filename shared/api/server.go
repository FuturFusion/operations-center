package api

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"
)

type ServerType string

const (
	ServerTypeUnknown          ServerType = "unknown"
	ServerTypeIncus            ServerType = "incus"
	ServerTypeMigrationManager ServerType = "migration-manager"
	ServerTypeOperationsCenter ServerType = "operations-center"
)

var serverTypes = map[ServerType]struct{}{
	ServerTypeUnknown:          {},
	ServerTypeIncus:            {},
	ServerTypeMigrationManager: {},
	ServerTypeOperationsCenter: {},
}

// MarshalText implements the encoding.TextMarshaler interface.
func (s ServerType) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *ServerType) UnmarshalText(text []byte) error {
	_, ok := serverTypes[ServerType(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid server type", string(text))
	}

	*s = ServerType(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (s ServerType) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface.
func (s *ServerType) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid server type")
	}

	switch v := value.(type) {
	case string:
		return s.UnmarshalText([]byte(v))
	case []byte:
		return s.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for server type", value)
	}
}

// Server defines a server running Hypervisor OS.
//
// swagger:model
type Server struct {
	// An opaque integer identifier for the server.
	// Example: 1
	ID int `json:"id" yaml:"id"`

	// The custer ID of the cluster the server is part of.
	// Example: 1
	ClusterID int `json:"cluster_id" yaml:"cluster_id"`

	// Name or name of the server.
	// Example: incus.local
	Name string `json:"name" yaml:"name"`

	// Type defines the type of the server, which is normally one of "incus", "migration-manager", "operations-center".
	// Example: incus
	Type ServerType `json:"server_type" yaml:"server_type"`

	// URL, hostname or IP address of the server endpoint.
	// Example: https://incus.local:6443
	ConnectionURL string `json:"connection_url" yaml:"connection_url"`

	// HardwareData contains the hardware data of the server, in the same form as presented by Incus in the resource API.
	HardwareData incusapi.Resources `json:"hardware_data" yaml:"hardware_data"` // FIXME: should this be json.RawMessage?

	// VersionData contains information about the servers version.
	// Example: ...
	VersionData json.RawMessage `json:"version_data" yaml:"version_data"` // FIXME: it is not yet clear, how the structure of the version information will actually look like.

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}
