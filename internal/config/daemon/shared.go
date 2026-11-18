package config

import (
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// SharedConfig is the config shared by all cluster members. The global database stores it as JSON, so its tags and
// the tags of its sections are the stored format.
type SharedConfig struct {
	// Network configuration shared by all cluster members.
	Network system.NetworkGlobal `json:"network" yaml:"network"`

	// Security configuration shared by all cluster members.
	Security system.Security `json:"security" yaml:"security"`

	// System settings configuration shared by all cluster members.
	Settings system.SettingsGlobal `json:"settings" yaml:"settings"`

	// Updates configuration shared by all cluster members.
	Updates system.Updates `json:"updates" yaml:"updates"`
}
