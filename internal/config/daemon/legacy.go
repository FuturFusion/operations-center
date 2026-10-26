package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

// legacyConfig is the layout of the config file before it was split into a local and a shared part.
type legacyConfig struct {
	Network struct {
		system.Network       `yaml:",inline"`
		system.NetworkGlobal `yaml:",inline"`
	} `json:"network" yaml:"network"`

	Security system.Security `json:"security" yaml:"security"`

	Settings struct {
		system.Settings       `yaml:",inline"`
		system.SettingsGlobal `yaml:",inline"`
	} `json:"settings" yaml:"settings"`

	Updates system.Updates `json:"updates" yaml:"updates"`
}

func (l legacyConfig) shared() system.SharedConfig {
	return system.SharedConfig{
		Network:  l.Network.NetworkGlobal,
		Security: l.Security,
		Settings: l.Settings.SettingsGlobal,
		Updates:  l.Updates,
		Cluster: system.Cluster{
			ClusterPut: system.ClusterPut{
				OfflineThresholdSeconds: DefaultOfflineThresholdSeconds,
			},
		},
	}
}

// defaultShared returns the built in defaults of the shared config.
func defaultShared() (system.SharedConfig, error) {
	cfg := legacyConfig{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	if err != nil {
		return system.SharedConfig{}, fmt.Errorf("Failed to unmarshal built in default config: %w", err)
	}

	return cfg.shared(), nil
}

// loadProvisionalShared returns the shared config the database is initialized with if it does not hold one yet.
// These are the built in defaults overlaid with the shared part of the config file.
// A config file from before the split is backed up before it is rewritten with the local part only.
func loadProvisionalShared(env enver) (system.SharedConfig, error) {
	filename := filepath.Join(env.VarDir(), ConfigFilename)
	migratedFilename := filepath.Join(env.VarDir(), "migrated_"+ConfigFilename)

	contents, err := os.ReadFile(migratedFilename)
	if errors.Is(err, os.ErrNotExist) {
		contents, err = backupLegacyConfig(filename, migratedFilename)
	}

	if err != nil {
		return system.SharedConfig{}, err
	}

	cfg := legacyConfig{}

	err = yaml.Unmarshal(defaultConfig, &cfg)
	if err != nil {
		return system.SharedConfig{}, fmt.Errorf("Failed to unmarshal built in default config: %w", err)
	}

	err = yaml.Unmarshal(contents, &cfg)
	if err != nil {
		return system.SharedConfig{}, fmt.Errorf("Failed to unmarshal config %q: %w", filename, err)
	}

	return cfg.shared(), nil
}

// backupLegacyConfig returns the contents of the config file and copies it to the backup location if it is from before the split.
func backupLegacyConfig(filename string, backupFilename string) ([]byte, error) {
	contents, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	cfg := legacyConfig{}

	err = yaml.Unmarshal(contents, &cfg)
	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal config %q: %w", filename, err)
	}

	// The updates source is mandatory, so it is only missing if the file is from after the split.
	if cfg.Updates.Source == "" {
		return contents, nil
	}

	err = os.WriteFile(backupFilename, contents, 0o600)
	if err != nil {
		return nil, fmt.Errorf("Failed to back up config %q: %w", filename, err)
	}

	return contents, nil
}
