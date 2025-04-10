package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	RestServerAddr string `yaml:"-"`
	RestServerPort int    `yaml:"-"`

	ClientCertificateFilename string `yaml:"-"`
	ClientKeyFilename         string `yaml:"-"`

	GithubToken string `yaml:"github.token"`

	// An array of SHA256 certificate fingerprints that belong to trusted TLS clients.
	TrustedTLSClientCertFingerprints []string `yaml:"trusted_tls_client_cert_fingerprints"`

	// OIDC-specific configuration.
	OidcIssuer   string `yaml:"oidc.issuer"`
	OidcClientID string `yaml:"oidc.client.id"`
	OidcScope    string `yaml:"oidc.scopes"`
	OidcAudience string `yaml:"oidc.audience"`
	OidcClaim    string `yaml:"oidc.claim"`
}

func (c *Config) LoadConfig(path string) error {
	contents, err := os.ReadFile(filepath.Join(path, "config.yml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	return yaml.Unmarshal(contents, c)
}
