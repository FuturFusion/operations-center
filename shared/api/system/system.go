package system

// NetworkLocal represents the network configuration of this member.
//
// swagger:model
type NetworkLocal struct {
	NetworkLocalPut `yaml:",inline"`
}

// NetworkLocalPut represents the fields available for an update of the
// network configuration of this member.
//
// swagger:model
type NetworkLocalPut struct {
	// Address and port to bind the REST API to.
	RestServerAddress string `json:"rest_server_address" yaml:"rest_server_address"`

	// Address and port used for intra-cluster communication. Is set when enabling clustering, and cannot be changed once set.
	ClusterAddress string `json:"cluster_address" yaml:"cluster_address"`
}

// SettingsLocal represents the system settings of this member.
//
// swagger:model
type SettingsLocal struct {
	SettingsLocalPut `yaml:",inline"`
}

// SettingsLocalPut represents the fields available for an update of the
// system settings of this member.
//
// swagger:model
type SettingsLocalPut struct {
	// Daemon log level.
	LogLevel string `json:"log_level" yaml:"log_level"`

	// Log levels per component, overriding LogLevel. The keys are component
	// names, a level configured for a component applies to all of its children
	// as well.
	// Example: {"provisioning": "DEBUG"}
	LogLevels map[string]string `json:"log_levels" yaml:"log_levels"`

	// PprofEnabled enables the pprof debug endpoints under /internal/debug/pprof.
	// Example: false
	PprofEnabled bool `json:"pprof_enabled" yaml:"pprof_enabled"`

	// MetricsEnabled enables the metrics endpoint /1.0/metrics.
	// Example: true
	MetricsEnabled bool `json:"metrics_enabled" yaml:"metrics_enabled"`

	// MetricsComponents lists the components that expose metrics.
	// Enabling a component also enables all of its children.
	// Example: ["runtime"]
	MetricsComponents []string `json:"metrics_components" yaml:"metrics_components"`
}
