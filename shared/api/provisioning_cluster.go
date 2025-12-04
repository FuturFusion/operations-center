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
		return fmt.Errorf("null is not a valid cluster status")
	}

	switch v := value.(type) {
	case string:
		return s.UnmarshalText([]byte(v))
	case []byte:
		return s.UnmarshalText(v)
	default:
		return fmt.Errorf("type %T is not supported for cluster status", value)
	}
}

type ClusterDeleteMode string

const (
	ClusterDeleteModeNormal       ClusterDeleteMode = "normal"
	ClusterDeleteModeForce        ClusterDeleteMode = "force"
	ClusterDeleteModeFactoryReset ClusterDeleteMode = "factory-reset"
)

var ClusterDeleteModes = map[ClusterDeleteMode]struct{}{
	ClusterDeleteModeNormal:       {},
	ClusterDeleteModeForce:        {},
	ClusterDeleteModeFactoryReset: {},
}

func (s ClusterDeleteMode) String() string {
	return string(s)
}

// Cluster defines a cluster of servers running Hypervisor OS.
//
// swagger:model
type Cluster struct {
	// A human-friendly name for this cluster.
	// Example: MyCluster
	Name string `json:"name" yaml:"name"`

	// URL, hostname or IP address of the cluster endpoint.
	// This is only user facing, e.g. the address of a load balancer infront of
	// the cluster and not used by Operations Center for direct communication
	// Operations Center relies on the connection URL of the cluster members.
	// Example: https://incus.local:6443
	ConnectionURL string `json:"connection_url" yaml:"connection_url"`

	// Certificate of the cluster endpoint in PEM encoded format.
	// Example:
	//	-----BEGIN CERTIFICATE-----
	//	...
	//	-----END CERTIFICATE-----
	Certificate string `json:"certificate" yaml:"certificate"`

	// Fingerprint in SHA256 format of the certificate.
	// Example: fd200419b271f1dc2a5591b693cc5774b7f234e1ff8c6b78ad703b6888fe2b69
	Fingerprint string `json:"fingerprint" yaml:"fingerprint"`

	// Status contains the status the cluster is currently in from the point of view of Operations Center.
	// Possible values for status are: pending, ready
	// Example: pending
	Status ClusterStatus `json:"status" yaml:"status"`

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

	// ServerType is the expected type of servers to be clustered.
	// Clustering will fail, if not all the servers are of the same type.
	ServerType ServerType `json:"server_type" yaml:"server_type"`

	// ServicesConfig contains the configuration for each service, which should be configured on Hypervisor OS.
	// Operations Center is simply passing forward the settings to Hypervisor OS.
	// For details about the configuration settings available refer to the service
	// API definitions in https://github.com/lxc/incus-os/tree/main/incus-osd/api.
	ServicesConfig map[string]any `json:"services_config" yaml:"services_config"`

	// ApplicationSeedConfig contains the seed configuration for the application, which is
	// applied during post clustering. This configuration is application specific.
	ApplicationSeedConfig map[string]any `json:"application_seed_config" yaml:"application_seed_config"`

	// ClusterTemplate contains the name of a cluster template, which should be
	// used for the cluster creation.
	// If ClusterTemplate is a none empty string, the respective cluster template
	// is used and the values in ServiceConfig and ApplicationConfig are
	// disregarded. If the cluster template is not found, an error is returned.
	ClusterTemplate string `json:"cluster_template" yaml:"cluster_template"`

	// ClusterTemplateVariableValues contains the variable values, which should
	// be applied to the respective placeholders in the cluster template.
	ClusterTemplateVariableValues ConfigMap `json:"cluster_template_variable_values" yaml:"cluster_template_variable_values"`
}

// ClusterCertificatePut represents the certificate and key pair for all cluster members.
//
// swagger:model
type ClusterCertificatePut struct {
	// The new certificate (X509 PEM encoded) for the cluster.
	// Example: X509 PEM certificate
	ClusterCertificate string `json:"cluster_certificate" yaml:"cluster_certificate"`

	// The new certificate key (X509 PEM encoded) for the cluster.
	// Example: X509 PEM certificate key
	ClusterCertificateKey string `json:"cluster_certificate_key" yaml:"cluster_certificate_key"`
}
