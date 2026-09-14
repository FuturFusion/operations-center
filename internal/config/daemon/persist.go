package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

// normalize fills in the values the daemon derives from what the user provided.
// It runs before every validation, so validation and persistence always see the
// same configuration.
func normalize(cfg config) (config, error) {
	var err error

	cfg.Network.NetworkPut, err = NetworkSetDefaults(cfg.Network.NetworkPut)
	if err != nil {
		return config{}, fmt.Errorf("Invalid network config: %w", err)
	}

	// Only apply ACME defaults if the mandatory settings are provided.
	if cfg.Security.ACME.Domain != "" && cfg.Security.ACME.Email != "" && cfg.Security.ACME.AgreeTOS {
		if cfg.Security.ACME.Challenge == "" {
			cfg.Security.ACME.Challenge = "HTTP-01"
		}

		if cfg.Security.ACME.Address == "" {
			cfg.Security.ACME.Address = ":80"
		}

		if cfg.Security.ACME.CAURL == "" {
			cfg.Security.ACME.CAURL = "https://acme-v02.api.letsencrypt.org/directory"
		}
	}

	// Setting updates.updates_default_channel can not be empty, use default value instead.
	if cfg.Updates.UpdatesDefaultChannel == "" {
		cfg.Updates.UpdatesDefaultChannel = "stable"
	}

	// Setting updates.server_default_channel can not be empty, use default value instead.
	if cfg.Updates.ServerDefaultChannel == "" {
		cfg.Updates.ServerDefaultChannel = "stable"
	}

	return cfg, nil
}

// loadConfig reads the built in defaults and overlays them with the config file,
// if there is one. It returns the raw file contents alongside, so the caller can
// tell whether persisting the normalized result would change anything.
func loadConfig(env enver) (config, []byte, error) {
	cfg := config{}

	err := yaml.Unmarshal(defaultConfig, &cfg)
	if err != nil {
		return config{}, nil, fmt.Errorf("Failed to unmarshal built in default config: %w", err)
	}

	filename := filepath.Join(env.VarDir(), ConfigFilename)

	contents, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil, nil
		}

		return config{}, nil, err
	}

	err = yaml.Unmarshal(contents, &cfg)
	if err != nil {
		return config{}, nil, fmt.Errorf("Failed to unmarshal config %q: %w", filename, err)
	}

	return cfg, contents, nil
}

func marshalConfig(cfg config) ([]byte, error) {
	buf := &bytes.Buffer{}

	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)

	err := enc.Encode(cfg)
	if err != nil {
		return nil, err
	}

	err = enc.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// configFileMode is the mode of the config file. It holds credentials, so it
// must not be world readable.
const configFileMode = 0o600

// ensureConfigFileMode tightens the permissions of an existing config file.
// Config files written before the daemon started doing so are world readable.
func ensureConfigFileMode(env enver) error {
	filename := filepath.Join(env.VarDir(), ConfigFilename)

	fileInfo, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if fileInfo.Mode().Perm() == configFileMode {
		return nil
	}

	err = os.Chmod(filename, configFileMode)
	if err != nil {
		return fmt.Errorf("Failed to set permissions on config %q: %w", filename, err)
	}

	return nil
}

// saveToDisk writes the configuration to the config file. The write is atomic:
// the new contents are written to a temporary file in the same directory, which
// then replaces the config file, so a failure can never leave a truncated config
// behind. It has no effect on the in memory configuration.
func saveToDisk(env enver, cfg config) error {
	contents, err := marshalConfig(cfg)
	if err != nil {
		return err
	}

	return file.WriteFileAtomic(filepath.Join(env.VarDir(), ConfigFilename), contents, configFileMode)
}
