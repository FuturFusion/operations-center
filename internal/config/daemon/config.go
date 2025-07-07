package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	RestServerAddr string `yaml:"-"`
	RestServerPort int    `yaml:"-"`

	ClientCertificateFilename string `yaml:"-"`
	ClientKeyFilename         string `yaml:"-"`

	// update.source is the URL of the origin, the updates should be fetched from.
	// If update.source starts with https://github.com/, the Github client is used
	// to fetch the updates from https://github.com/lxc/incus-os.
	UpdatesSource              string        `yaml:"update.source"`
	UpdatesSourcePollInterval  time.Duration `yaml:"-"`
	UpdatesSourcePollSkipFirst bool          `yaml:"update.source_skip_first_update"`
	GithubToken                string        `yaml:"github.token"`
	// Root CA certificate used to verify the signature of index.sjson.
	UpdateSignatureVerificationRootCA string `yaml:"update.signature_verification_root_ca"`

	ConnectivityCheckInterval time.Duration `yaml:"-"`
	PendingServerPollInterval time.Duration `yaml:"-"`

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
