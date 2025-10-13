package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type AuthType string

const (
	AuthTypeUntrusted = AuthType("untrusted")
	AuthTypeTLS       = AuthType("tls")
	AuthTypeOIDC      = AuthType("oidc")
)

var authTypes = map[AuthType]struct{}{
	AuthTypeUntrusted: {},
	AuthTypeTLS:       {},
	AuthTypeOIDC:      {},
}

type Config struct {
	// Config from global flags
	Verbose    bool `yaml:"-"`
	Debug      bool `yaml:"-"`
	ForceLocal bool `yaml:"-"`

	DefaultRemote string            `yaml:"default_remote"`
	Remotes       map[string]Remote `yaml:"remotes"`

	TLSClientCertFile string `yaml:"tls_client_cert_file"`
	TLSClientKeyFile  string `yaml:"tls_client_key_file"`
}

type Remote struct {
	Addr     string   `yaml:"addr"`
	AuthType AuthType `yaml:"auth_type"`
}

func (c *Config) LoadConfig(path string) error {
	contents, err := os.ReadFile(filepath.Join(path, "config.yml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	err = yaml.Unmarshal(contents, c)
	if err != nil {
		return err
	}

	for remote, config := range c.Remotes {
		if config.AuthType == "" {
			config.AuthType = AuthTypeUntrusted
		}

		_, ok := authTypes[config.AuthType]
		if !ok {
			return fmt.Errorf("Invalid value for config key auth_type: %v", config.AuthType)
		}

		c.Remotes[remote] = config
	}

	return nil
}

func (c *Config) SaveConfig(path string) error {
	body, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(path, "config.yml"), body, 0o600)
	if err != nil {
		return err
	}

	return nil
}
