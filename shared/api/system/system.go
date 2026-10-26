package system

// Network represents the system's network configuration.
//
// swagger:model
type Network struct {
	NetworkPut `yaml:",inline"`
}

// NetworkPut represents the fields available for an update of the
// system's network configuration.
//
// swagger:model
type NetworkPut struct {
	// Address and port to bind the REST API to.
	RestServerAddress string `json:"rest_server_address" yaml:"rest_server_address"`

	// Address and port used for intra-cluster communication. Is set when enabling clustering, and cannot be changed once set.
	ClusterAddress string `json:"cluster_address" yaml:"cluster_address"`
}

// Settings represents global system settings.
//
// swagger:model
type Settings struct {
	SettingsPut `yaml:",inline"`
}

// SettingsPut represents the fields available for an update of the global
// system settings.
//
// swagger:model
type SettingsPut struct {
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

	// MetricsComponents is the list of components, which expose metrics.
	// Enabling a component enables all of its children as well.
	// Example: ["runtime", "api"]
	MetricsComponents []string `json:"metrics_components" yaml:"metrics_components"`
}
