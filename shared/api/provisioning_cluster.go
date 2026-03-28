package api

import (
	"database/sql/driver"
	"encoding/json"
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

type ClusterUpdateInProgressStatus struct {
	// InProgress is true, if a cluster wide update is on going and false
	// otherwise.
	// Example: true
	InProgress ClusterUpdateInProgress `json:"in_progress" yaml:"in_progress"`

	// StatusDescription contains progress information for the user in plain text
	// form.
	// Example: [3/20] Evacuating server xyz
	StatusDescription *string `json:"status_description,omitempty" yaml:"status_description"`

	// EvacuatedBefore contains the list of server names of the servers, that have
	// been manually evacuated already before the rolling update has been
	// triggered.
	EvacuatedBefore []string `json:"evacuated_before" yaml:"evacuated_before"`

	// LastUpdated is the time, when this information has been updated for the
	// last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}

type ClusterUpdateInProgress string

const (
	ClusterUpdateInProgressInactive              ClusterUpdateInProgress = ""
	ClusterUpdateInProgressApplyUpdate           ClusterUpdateInProgress = "applying updates"
	ClusterUpdateInProgressApplyUpdateWithReboot ClusterUpdateInProgress = "applying updates with reboot"
	ClusterUpdateInProgressRollingRestart        ClusterUpdateInProgress = "restarting servers"
)

// ClusterUpdateStatus contains the update status of each server of the cluster
// as well as an aggregated cluster update status.
type ClusterUpdateStatus struct {
	// NeedsUpdate holds the list of server names of the servers within the
	// cluster, which need to be updated. If NeedsUpdate is empty, all servers are
	// up to date.
	NeedsUpdate []string `json:"needs_update,omitempty" yaml:"needs_update"`

	// NeedsReboot holds the list of server names of the servers within the
	// cluster, which need to be rebooted. If NeedsReboot is empty, all servers
	// are up to date and don't require a reboot.
	NeedsReboot []string `json:"needs_reboot,omitempty" yaml:"needs_reboot"`

	// InMaintenance holds the list of server names of the servers within the
	// cluster, which are currently in a maintenance state other than
	// `NotInMaintenance`. If InMaintenance is empty, all servers are fully
	// operational and not in maintenance.
	InMaintenance []string `json:"in_maintenance,omitempty" yaml:"in_maintenance"`

	// InProgressStatus holds the status information about an ongoing cluster
	// update, if any.
	InProgressStatus ClusterUpdateInProgressStatus `json:"in_progress_status" yaml:"in_progress_status"`
}

// Value implements the sql driver.Valuer interface.
func (c ClusterUpdateStatus) Value() (driver.Value, error) {
	// Don't persist calculated fields in the DB.
	clusterUpdateStatus := c

	clusterUpdateStatus.NeedsUpdate = nil
	clusterUpdateStatus.NeedsReboot = nil
	clusterUpdateStatus.InMaintenance = nil
	clusterUpdateStatus.InProgressStatus.StatusDescription = nil

	return json.Marshal(clusterUpdateStatus)
}

// Scan implements the sql.Scanner interface.
func (c *ClusterUpdateStatus) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid cluster update in progress status")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*c = ClusterUpdateStatus{}
			return nil
		}

		return json.Unmarshal([]byte(v), c)

	case []byte:
		if len(v) == 0 {
			*c = ClusterUpdateStatus{}
			return nil
		}

		return json.Unmarshal(v, c)

	default:
		return fmt.Errorf("type %T is not supported for cluster update in progress status", value)
	}
}

type ClusterConfigRollingRestart struct {
	// PostRestoreDelay holds the time, that is waited between the resore
	// of a server and the evacuation of the next server. This should be set if
	// RestoreMode is kept at the default value in order to grant a cluster
	// enough time to move previously evacuated instances back to their
	// originating server.
	PostRestoreDelay time.Duration `json:"post_restore_delay" yaml:"post_restore_delay"`

	// RestoreMode is the mode applied dring Incus restore operation. Valid
	// values are "" (default, move instances back, that have been evacuated
	// previously) and "skip" (skip moving evacuated instances back).
	// Example: skip
	RestoreMode string `json:"restore_mode" yaml:"restore_mode"`
}

// ClusterConfig contains cluster wide configuration used by Operations Center
// when interacting with the cluster.
type ClusterConfig struct {
	RollingRestart ClusterConfigRollingRestart `json:"rolling_restart" yaml:"rolling_restart"`
}

func (c ClusterConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *ClusterConfig) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid cluster config")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*c = ClusterConfig{}
			return nil
		}

		return json.Unmarshal([]byte(v), c)

	case []byte:
		if len(v) == 0 {
			*c = ClusterConfig{}
			return nil
		}

		return json.Unmarshal(v, c)

	default:
		return fmt.Errorf("type %T is not supported for cluster config", value)
	}
}

// ClusterPut defines the updateable part of a cluster of servers running
// Hypervisor OS.
//
// swagger:model
type ClusterPut struct {
	// URL, hostname or IP address of the cluster endpoint.
	// This is only user facing, e.g. the address of a load balancer infront of
	// the cluster and not used by Operations Center for direct communication
	// Operations Center relies on the connection URL of the cluster members.
	// Example: https://incus.local:6443
	ConnectionURL string `json:"connection_url" yaml:"connection_url"`

	// Channel the cluster is following for updates.
	// Example: stable
	Channel string `json:"channel" yaml:"channel"`

	// Description of the cluster.
	// Example: Lab cluster with limited resources.
	Description string `json:"description" yaml:"description"`

	// Properties contains properties of the cluster as key/value pairs.
	// Example (in YAML notation for readability):
	//   properties:
	//     env: lab
	Properties ConfigMap `json:"properties" yaml:"properties"`

	// Config contains cluster wide configuration used by Operations Center
	// when interacting with the cluster. For example waiting delays during
	// cluster evacuation and restore, before a subsequent server is processed.
	Config ClusterConfig `json:"config" yaml:"config"`
}

// Cluster defines a cluster of servers running Hypervisor OS.
//
// swagger:model
type Cluster struct {
	ClusterPut `yaml:",inline"`

	// A human-friendly name for this cluster.
	// Example: MyCluster
	Name string `json:"name" yaml:"name"`

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

	// UpdateStatus contains the aggregated update state for the cluster,
	// which consists of the lowest state of all the servers of the cluster.
	//
	// Additionally, it contains details about an ongoing cluster wide update
	// process if any.
	UpdateStatus ClusterUpdateStatus `json:"update_status" yaml:"update_status"`

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

type ClusterBulkUpdateAction string

const (
	ClusterBulkUpdateActionInvalid                        ClusterBulkUpdateAction = ""
	ClusterBulkUpdateActionAddNetworkInterfaceVLANTags    ClusterBulkUpdateAction = "add_network_interface_vlan_tags"
	ClusterBulkUpdateActionRemoveNetworkInterfaceVLANTags ClusterBulkUpdateAction = "remove_network_interface_vlan_tags"
	ClusterBulkUpdateActionUpdateSystemLogging            ClusterBulkUpdateAction = "update_system_logging"
	ClusterBulkUpdateActionUpdateSystemKernel             ClusterBulkUpdateAction = "update_system_kernel"
	ClusterBulkUpdateActionAddApplication                 ClusterBulkUpdateAction = "add_application"
	ClusterBulkUpdateActionAddISCSIStorageTarget          ClusterBulkUpdateAction = "add_iscsi_storage_target"
	ClusterBulkUpdateActionRemoveISCSIStorageTarget       ClusterBulkUpdateAction = "remove_iscsi_storage_target"
	ClusterBulkUpdateActionAddMultipathStorageTarget      ClusterBulkUpdateAction = "add_multipath_storage_target"
	ClusterBulkUpdateActionRemoveMultipathStorageTarget   ClusterBulkUpdateAction = "remove_multipath_storage_target"
	ClusterBulkUpdateActionAddNVMEStorageTarget           ClusterBulkUpdateAction = "add_nvme_storage_target"
	ClusterBulkUpdateActionRemoveNVMEStorageTarget        ClusterBulkUpdateAction = "remove_nvme_storage_target"
)

// ClusterBulkUpdatePost represents a cluster bulk update request containing
// action and optional arguments.
//
// swagger:model
type ClusterBulkUpdatePost struct {
	// Action to be executed for this bulk update.
	Action ClusterBulkUpdateAction `json:"action" yaml:"action"`

	// Arguments for the action, the exact structure depends on the
	// defined action.
	Arguments *json.RawMessage `json:"arguments" yaml:"arguments"`
}

// ClusterUpdatePost represents a cluster update request.
//
// swagger:model
type ClusterUpdatePost struct {
	// Reboot indicates ifif after the update a rolling reboot of the servers
	// should be triggered or not. If reboot is set to true, the servers are
	// rebooted, otherwise only the updates are applied without reboot (OS updates
	// will require a reboot at a later stage).
	Reboot bool `json:"reboot" yaml:"reboot"`
}
