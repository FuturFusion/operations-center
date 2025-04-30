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

	AuthType               AuthType `yaml:"auth_type"`
	OperationsCenterServer string   `yaml:"operations_center_server"`
	TLSClientCertFile      string   `yaml:"tls_client_cert_file"`
	TLSClientKeyFile       string   `yaml:"tls_client_key_file"`
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

	_, ok := authTypes[c.AuthType]
	if !ok {
		return fmt.Errorf("Invalid value for config key auth_type: %v", c.AuthType)
	}

	return nil
}
