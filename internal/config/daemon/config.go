package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	OperationsCenterAddress string `yaml:"address"`
	RestServerAddr          string `yaml:"-"`
	RestServerPort          int    `yaml:"-"`

	ClientCertificateFilename string `yaml:"-"`
	ClientKeyFilename         string `yaml:"-"`

	// update.source is the URL of the origin, the updates should be fetched from.
	UpdatesSource              string        `yaml:"update.source"`
	UpdatesSourcePollInterval  time.Duration `yaml:"-"`
	UpdatesSourcePollSkipFirst bool          `yaml:"update.source_skip_first_update"`
	// Root CA certificate used to verify the signature of index.sjson.
	UpdateSignatureVerificationRootCA string `yaml:"update.signature_verification_root_ca"`

	ConnectivityCheckInterval time.Duration `yaml:"-"`
	PendingServerPollInterval time.Duration `yaml:"-"`

	InventoryUpdateInterval time.Duration `yaml:"-"`

	// An array of SHA256 certificate fingerprints that belong to trusted TLS clients.
	TrustedTLSClientCertFingerprints []string `yaml:"trusted_tls_client_cert_fingerprints"`

	// OIDC-specific configuration.
	OidcIssuer   string `yaml:"oidc.issuer"`
	OidcClientID string `yaml:"oidc.client.id"`
	OidcScope    string `yaml:"oidc.scopes"`
	OidcAudience string `yaml:"oidc.audience"`
	OidcClaim    string `yaml:"oidc.claim"`

	// OpenFGA-specific configuration.
	OpenfgaAPIToken string `yaml:"openfga.api.token"`
	OpenfgaAPIURL   string `yaml:"openfga.api.url"`
	OpenfgaStoreID  string `yaml:"openfga.store.id"`
}

//go:embed default.yml
var defaultConfig []byte

func (c *Config) LoadConfig(path string) error {
	err := yaml.Unmarshal(defaultConfig, c)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal built in default config: %w", err)
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

	// Validate config.
	if c.OperationsCenterAddress == "" {
		return fmt.Errorf(`Invalid config, "address" property can not be empty`)
	}

	return nil
}
