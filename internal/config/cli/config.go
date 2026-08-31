package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	incusTLS "github.com/lxc/incus/v7/shared/tls"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/shared/api"
)

type AuthType string

const (
	AuthTypeUntrusted = AuthType(api.AuthenticationUntrusted)
	AuthTypeTLS       = AuthType(api.AuthenticationMethodTLS)
	AuthTypeOIDC      = AuthType(api.AuthenticationMethodOIDC)
)

var authTypes = map[AuthType]struct{}{
	AuthTypeUntrusted: {},
	AuthTypeTLS:       {},
	AuthTypeOIDC:      {},
}

type Config struct {
	// Config from global flags
	Verbose    bool   `json:"-" yaml:"-"`
	Debug      bool   `json:"-" yaml:"-"`
	ForceLocal bool   `json:"-" yaml:"-"`
	ConfigDir  string `json:"-" yaml:"-"`

	DefaultRemote string            `json:"default_remote" yaml:"default_remote"`
	Remotes       map[string]Remote `json:"remotes" yaml:"remotes"`

	CertInfo *incusTLS.CertInfo `json:"-" yaml:"-"`
}

type Remote struct {
	Addr       string          `json:"addr" yaml:"addr"`
	AuthType   AuthType        `json:"auth_type" yaml:"auth_type"`
	ServerCert api.Certificate `json:"server_cert" yaml:"server_cert"`
}

func (c *Config) LoadConfig(path string) error {
	err := os.MkdirAll(filepath.Join(path, "oidc-tokens"), 0o700)
	if err != nil {
		return err
	}

	c.ConfigDir = path

	c.CertInfo, err = incusTLS.KeyPairAndCA(path, "client", incusTLS.CertClient, false)
	if err != nil {
		return fmt.Errorf("Failed to create client certificate: %w", err)
	}

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

func (c *Config) SaveConfig() error {
	body, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(c.ConfigDir, "config.yml"), body, 0o600)
	if err != nil {
		return err
	}

	return nil
}
