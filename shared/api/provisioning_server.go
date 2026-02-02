package api

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
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

func (s ServerType) String() string {
	return string(s)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (s ServerType) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *ServerType) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*s = ServerTypeUnknown
		return nil
	}

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

type ServerStatus string

const (
	ServerStatusUnknown ServerStatus = "unknown"
	ServerStatusPending ServerStatus = "pending"
	ServerStatusReady   ServerStatus = "ready"
)

var serverStatuses = map[ServerStatus]struct{}{
	ServerStatusUnknown: {},
	ServerStatusPending: {},
	ServerStatusReady:   {},
}

func (s ServerStatus) String() string {
	return string(s)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (s ServerStatus) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *ServerStatus) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*s = ServerStatusUnknown
		return nil
	}

	_, ok := serverStatuses[ServerStatus(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid server status", string(text))
	}

	*s = ServerStatus(text)

	return nil
}

// Value implements the sql driver.Valuer interface.
func (s ServerStatus) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface.
func (s *ServerStatus) Scan(value any) error {
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

type HardwareData struct {
	incusapi.Resources
}

// Value implements the sql driver.Valuer interface.
func (h HardwareData) Value() (driver.Value, error) {
	return json.Marshal(h)
}

// Scan implements the sql.Scanner interface.
func (h *HardwareData) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid hardware data")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*h = HardwareData{}
			return nil
		}

		return json.Unmarshal([]byte(v), h)

	case []byte:
		if len(v) == 0 {
			*h = HardwareData{}
			return nil
		}

		return json.Unmarshal(v, h)

	default:
		return fmt.Errorf("type %T is not supported for hardware data", value)
	}
}

type OSData struct {
	// Network contains the network data of the server OS, in the same form as presented by IncusOS in the network API.
	Network incusosapi.SystemNetwork `json:"network" yaml:"network"`

	// Security contains the security data of the server OS, in the same form as presented by IncusOS in the security API.
	Security incusosapi.SystemSecurity `json:"security" yaml:"security"`

	// Storage contains the storage data of the server OS, in the same form as presented by IncusOS in the storage API.
	Storage incusosapi.SystemStorage `json:"storage" yaml:"storage"`
}

// Value implements the sql driver.Valuer interface.
func (h OSData) Value() (driver.Value, error) {
	return json.Marshal(h)
}

// Scan implements the sql.Scanner interface.
func (h *OSData) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid OS data")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*h = OSData{}
			return nil
		}

		return json.Unmarshal([]byte(v), h)

	case []byte:
		if len(v) == 0 {
			*h = OSData{}
			return nil
		}

		return json.Unmarshal(v, h)

	default:
		return fmt.Errorf("type %T is not supported for OS data", value)
	}
}

// ServerVersionData defines the version information for a server including
// the OS and all its applications.
//
// swagger:model
type ServerVersionData struct {
	// OS holds the version information for the operating system.
	OS OSVersionData `json:"os" yaml:"os"`

	// Applications holds the version information for the installed applications.
	Applications []ApplicationVersionData `json:"applications" yaml:"applications"`

	// The channel the system is following for updates.
	UpdateChannel string `json:"update_channel" yaml:"update_channel"`
}

// OSVersionData defines a single version information for the OS.
//
// swagger:model
type OSVersionData struct {
	// Name of the software component.
	// Example: IncusOS
	Name string `json:"name" yaml:"name"`

	// Version string.
	// Example: 202512250102
	Version string `json:"version" yaml:"version"`

	// Next Version string. If this version is different from "version",
	// an update is available and applied on the system, but the system has
	// not yet been rebooted, so the new update is not yet active.
	// Example: 202512250102
	VersionNext string `json:"version_next" yaml:"version_next"`

	// AvailableVersion is the most recent version available for the OS in the
	// update channel assigned to the respective system.
	AvailableVersion *string `json:"available_version,omitempty" yaml:"available_version,omitempty"`

	// NeedsReboot is the "needs_reboot" state reported by the server. Currently
	// this is only expected to be "true", if "version_next" is different than
	// "version", but in the future, there might be other reasons for a server
	// to report, that a reboot is required.
	NeedsReboot bool `json:"needs_reboot" yaml:"needs_reboot"`

	// NeedsUpdate is true, if the OS needs to be updated
	// (available_version > version).
	NeedsUpdate *bool `json:"needs_update,omitempty" yaml:"needs_update,omitempty"`
}

// ApplicationVersionData defines a single version information for an application.
//
// swagger:model
type ApplicationVersionData struct {
	// Name of the software component.
	// Example: IncusOS
	Name string `json:"name" yaml:"name"`

	// Version string.
	// Example: 202512250102
	Version string `json:"version" yaml:"version"`

	// AvailableVersion is the most recent version available for this application
	// in the update channel assigned to the respective system.
	AvailableVersion *string `json:"available_version,omitempty" yaml:"available_version,omitempty"`

	// NeedsUpdate is true, if this application needs to be updated
	// (available_version > version).
	NeedsUpdate *bool `json:"needs_update,omitempty" yaml:"needs_update,omitempty"`
}

// Value implements the sql driver.Valuer interface.
func (s ServerVersionData) Value() (driver.Value, error) {
	// Don't persist AvailableVersion and NeedsUpdate in the DB.
	s.OS.AvailableVersion = nil
	s.OS.NeedsUpdate = nil
	for i := range s.Applications {
		s.Applications[i].AvailableVersion = nil
		s.Applications[i].NeedsUpdate = nil
	}

	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface.
func (s *ServerVersionData) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid server version data")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*s = ServerVersionData{}
			return nil
		}

		return json.Unmarshal([]byte(v), s)

	case []byte:
		if len(v) == 0 {
			*s = ServerVersionData{}
			return nil
		}

		return json.Unmarshal(v, s)

	default:
		return fmt.Errorf("type %T is not supported for server version data", value)
	}
}

// ServerPost defines a new server running Hypervisor OS.
//
// swagger:model
type ServerPost struct {
	ServerPut `yaml:",inline"`

	// Name of the server.
	// Example: incus.local
	Name string `json:"name" yaml:"name"`

	// URL, hostname or IP address of the server endpoint used by Operations
	// Center for its communication.
	// Example: https://incus.local:6443
	ConnectionURL string `json:"connection_url" yaml:"connection_url"`
}

// ServerPut defines the updateable part of a server running Hypervisor OS.
//
// swagger:model
type ServerPut struct {
	// Public URL, hostname or IP address of the server endpoint for user facing
	// communication with the server. Only required, if it differs from
	// connection_url, e.g. because the server is behind a reverse proxy.
	// Example: https://incus.local:6443
	PublicConnectionURL string `json:"public_connection_url" yaml:"public_connection_url"`

	// Channel the server is following for updates.
	// Example: stable
	Channel string `json:"channel" yaml:"channel"`
}

// Server defines a server running Hypervisor OS.
//
// swagger:model
type Server struct {
	ServerPost `yaml:",inline"`

	// Certificate of the server endpoint in PEM encoded format.
	// Example:
	//	-----BEGIN CERTIFICATE-----
	//	...
	//	-----END CERTIFICATE-----
	Certificate string `json:"certificate" yaml:"certificate"`

	// Fingerprint in SHA256 format of the certificate.
	// Example: fd200419b271f1dc2a5591b693cc5774b7f234e1ff8c6b78ad703b6888fe2b69
	Fingerprint string `json:"fingerprint" yaml:"fingerprint"`

	// The cluster the server is part of.
	// Example: one
	Cluster string `json:"cluster" yaml:"cluster"`

	// Type defines the type of the server, which is normally one of "incus", "migration-manager", "operations-center".
	// Example: incus
	Type ServerType `json:"server_type" yaml:"server_type"`

	// HardwareData contains the hardware data of the server, in the same form as presented by Incus in the resource API.
	HardwareData HardwareData `json:"hardware_data" yaml:"hardware_data"`

	// OSData contains the configuration data of the operating system, e.g. incus-os.
	OSData OSData `json:"os_data" yaml:"os_data"`

	// VersionData contains information about the servers version.
	VersionData ServerVersionData `json:"version_data" yaml:"version_data"`

	// Status contains the status the server is currently in from the point of view of Operations Center.
	// Possible values for status are: pending, ready
	// Example: pending
	Status ServerStatus `json:"server_status" yaml:"server_status"`

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`

	// LastSeen is the time, when this server has been seen for the last time
	// by any sort of connection between the server and operations center
	// in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastSeen time.Time `json:"last_seen" yaml:"last_seen"`
}

// ServerSelfUpdate defines a self update request of a server.
//
// swagger:model
type ServerSelfUpdate struct {
	// URL, hostname or IP address of the server endpoint.
	// Example: https://incus.local:6443
	ConnectionURL string `json:"connection_url" yaml:"connection_url"`
}

// ServerRegistrationResponse defines the response to a successful server registration.
type ServerRegistrationResponse struct {
	// ClientCertificate is the certificate in PEM format used by Operations Center
	// when connecting to servers or clusters.
	ClientCertificate string `json:"certificate" yaml:"certificate"`
}

// ServerSystemNetwork is a type alias to hold the system network configuration from IncusOS.
type ServerSystemNetwork = incusosapi.SystemNetwork

// ServerSystemStorage is a type alias to hold the system network configuration from IncusOS.
type ServerSystemStorage = incusosapi.SystemStorage

// ServerSystemProvider is a type alias to hold the system provider configuration from IncusOS.
type ServerSystemProvider = incusosapi.SystemProvider

// ServerSystemUpdate is a type alias to hold the system update configuration from IncusOS.
type ServerSystemUpdate = incusosapi.SystemUpdate
