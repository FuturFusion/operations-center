package provisioning

import (
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v6/shared/api"
)

// These types are taken from https://github.com/lxc/incus-os/blob/main/incus-osd/internal/seed/
// TODO: Decicde, if these types should be exported in https://github.com/lxc/incus-os/incus-osd or if we duplicate this data.

// Application represents an application.
type Application struct {
	Name string `json:"name" yaml:"name"`
}

// Applications represents a list of application.
type Applications struct {
	Applications []Application `json:"applications" yaml:"applications"`
	Version      string        `json:"version"      yaml:"version"`
}

// IncusConfig is a wrapper around the Incus preseed.
type IncusConfig struct {
	Version string `json:"version" yaml:"version"`

	ApplyDefaults bool `json:"apply_defaults" yaml:"apply_defaults"`

	Preseed *incusapi.InitPreseed `json:"preseed" yaml:"preseed"`
}

// InstallSeed defines a struct to hold install configuration.
type InstallSeed struct {
	Version string `json:"version" yaml:"version"`

	ForceInstall bool               `json:"force_install" yaml:"force_install"` // If true, ignore any existing data on target install disk.
	ForceReboot  bool               `json:"force_reboot"  yaml:"force_reboot"`  // If true, reboot the system automatically upon completion rather than waiting for the install media to be removed.
	Target       *InstallSeedTarget `json:"target"        yaml:"target"`        // Optional selector for the target install disk; if not set, expect a single drive to be present.
}

// InstallSeedTarget defines options used to select the target install disk.
type InstallSeedTarget struct {
	ID string `json:"id" yaml:"id"` // Name as listed in /dev/disk/by-id/, glob supported.
}

// NetworkSeed defines a struct to hold network configuration.
type NetworkSeed struct {
	incusosapi.SystemNetworkConfig `yaml:",inline"`

	Version string `json:"version" yaml:"version"`
}

// ProviderSeed defines a struct to hold provider configuration.
type ProviderSeed struct {
	incusosapi.SystemProviderConfig `yaml:",inline"`

	Version string `json:"version" yaml:"version"`
}
