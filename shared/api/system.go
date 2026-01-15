package api

import "fmt"

// SystemCertificatePost represents the fields available for an update of the
// system certificate (server certificate) and key.
//
// swagger:model
type SystemCertificatePost struct {
	// The new certificate (X509 PEM encoded) for the system (server certificate).
	// Example: X509 PEM certificate
	Certificate string `json:"certificate" yaml:"certificate"`

	// The new certificate key (X509 PEM encoded) for the system (server key).
	// Example: X509 PEM certificate key
	Key string `json:"key" yaml:"key"`
}

// SystemNetwork represents the system's network configuration.
//
// swagger:model
type SystemNetwork struct {
	SystemNetworkPut `yaml:",inline"`
}

// SystemNetworkPut represents the fields available for an update of the
// system's network configuration.
//
// swagger:model
type SystemNetworkPut struct {
	// Address of Operations Center which is used by managed servers to connect.
	OperationsCenterAddress string `json:"address" yaml:"address"`

	// Address and port to bind the REST API to.
	RestServerAddress string `json:"rest_server_address" yaml:"rest_server_address"`
}

// SystemSecurity represents the system's security configuration.
//
// swagger:model
type SystemSecurity struct {
	SystemSecurityPut `yaml:",inline"`
}

// SystemSecurityPut represents the fields available for an update of the
// system's security configuration.
//
// swagger:model
type SystemSecurityPut struct {
	// OIDC configuration.
	OIDC SystemSecurityOIDC `json:"oidc" yaml:"oidc"`

	// OpenFGA configuration.
	OpenFGA SystemSecurityOpenFGA `json:"openfga" yaml:"openfga"`

	// ACME configuration.
	ACME SystemSecurityACME `json:"acme" yaml:"acme"`

	// An array of SHA256 certificate fingerprints that belong to trusted TLS clients.
	TrustedTLSClientCertFingerprints []string `json:"trusted_tls_client_cert_fingerprints" yaml:"trusted_tls_client_cert_fingerprints"`

	// An array of trusted HTTPS proxy addresses.
	TrustedHTTPSProxies []string `json:"trusted_https_proxies" yaml:"trusted_https_proxies"`
}

// SystemSecurityOIDC is the OIDC related part of the system's security
// configuration.
type SystemSecurityOIDC struct {
	// OIDC Issuer.
	Issuer string `json:"issuer" yaml:"issuer"`

	// CLient ID used for communication with the OIDC issuer.
	ClientID string `json:"client_id" yaml:"client_id"`

	// Scopes to be requested.
	Scope string `json:"scopes" yaml:"scopes"`

	// Audience the OIDC tokens should be verified against.
	Audience string `json:"audience" yaml:"audience"`

	// Claim which should be used to identify the user or subject.
	Claim string `json:"claim" yaml:"claim"`
}

// SystemSecurityOpenFGA is the OpenFGA related part of the system's security
// configuration.
type SystemSecurityOpenFGA struct {
	// API token used for communication with the OpenFGA system.
	APIToken string `json:"api_token" yaml:"api_token"`

	// URL of the OpenFGA API.
	APIURL string `json:"api_url" yaml:"api_url"`

	// ID of the OpenFGA store.
	StoreID string `json:"store_id" yaml:"store_id"`
}

// ACMEChallengeType represents challenge types for ACME configuration.
type ACMEChallengeType string

const (
	// ACMEChallengeHTTP is the HTTP ACME challenge type.
	ACMEChallengeHTTP ACMEChallengeType = "HTTP-01"

	// ACMEChallengeDNS is the DNS ACME challenge type.
	ACMEChallengeDNS ACMEChallengeType = "DNS-01"
)

func (a ACMEChallengeType) Validate() error {
	switch a {
	case ACMEChallengeDNS:
	case ACMEChallengeHTTP:
	default:
		return fmt.Errorf("Unknown ACME challenge type %q", a)
	}

	return nil
}

type SystemSecurityACME struct {
	// Agree to ACME terms of service.
	AgreeTOS bool `json:"agree_tos" yaml:"agree_tos"`

	// CAURL holds the URL to the CA directory resource of the ACME service.
	CAURL string `json:"ca_url" yaml:"ca_url"`

	// Challenge holds the ACME challenge type to use.
	Challenge ACMEChallengeType `json:"challenge" yaml:"challenge"`

	// Domain for which the certificate is issued.
	Domain string `json:"domain" yaml:"domain"`

	// Email address used for the account registration.
	Email string `json:"email" yaml:"email"`

	// Address and interface for HTTP server (used by HTTP-01).
	Address string `json:"http_challenge_address" yaml:"http_challenge_address"`

	// Backend provider for the challenge (used by DNS-01)>
	Provider string `json:"provider" yaml:"provider"`

	// Environment variables to set during the challenge (used by DNS-01).
	ProviderEnvironment []string `json:"provider_environment" yaml:"provider_environment"`

	// List of DNS resolvers (used by DNS-01).
	ProviderResolvers []string `json:"provider_resolvers" yaml:"provider_resolvers"`
}

// SystemSettings represents global system settings.
//
// swagger:model
type SystemSettings struct {
	SystemSettingsPut `yaml:",inline"`
}

// SystemSettingsPut represents the fields available for an update of the global
// system settings.
//
// swagger:model
type SystemSettingsPut struct {
	// Daemon log level.
	LogLevel string `json:"log_level" yaml:"log_level"`
}

// SystemUpdates represents the system's updates configuration.
//
// swagger:model
type SystemUpdates struct {
	SystemUpdatesPut `yaml:",inline"`
}

// SystemUpdatesPut represents the fields available for an update of the
// system's updates configuration.
//
// swagger:model
type SystemUpdatesPut struct {
	// Source is the URL of the origin, the updates should be fetched from.
	Source string `json:"source" yaml:"source"`

	// Root CA certificate used to verify the signature of index.sjson.
	// Example: -----BEGIN CERTIFICATE-----\nMII...\n-----END CERTIFICATE-----
	SignatureVerificationRootCA string `json:"signature_verification_root_ca" yaml:"signature_verification_root_ca"`

	// Filter expression for updates using https://expr-lang.org/ on struct
	// provisioning.Update.
	// If a filter is defined, the filter needs to evaluate to true for the update
	// being fetched by Operations Center.
	// Empty filter expression does fallback to the default value defined below.
	// To disable filtering, set to "true", which causes the filter to allow all
	// updates.
	//
	// Default: 'stable' in upstream_channels
	//
	// Example: 'stable' in upstream_channels
	FilterExpression string `json:"filter_expression" yaml:"filter_expression"`

	// Filter expression for update files using https://expr-lang.org/ on struct
	// provisioning.UpdateFile.
	// If a filter is defined, the filter needs to evaluate to true for the file
	// being fetched by Operations Center.
	// Empty filter expression does fallback to the default value defined below.
	// To disable filtering, set to "true", which causes the filter to allow all
	// files.
	//
	// For file filter expression, the following helper functions are available:
	//   - applies_to_architecture(arch string, expected_arch ...string) bool
	//       Returns true if the 'arch' string matches one of the given
	//       'expected_arch' strings or if 'architecure' is not set.
	//
	// Default:
	//   applies_to_architecture(architecture, "x86_64")
	//
	// Examples:
	//   architecture == "x86_64"
	FileFilterExpression string `json:"file_filter_expression" yaml:"file_filter_expression"`

	// UpdatesDefaultChannel is the update channel, which is used by default
	// new updates fetched from upstream.
	UpdatesDefaultChannel string `json:"updates_default_channel" yaml:"updates_default_channel"`

	// ServerDefaultChannel is the default channel assigned to new server
	// and cluster instances.
	ServerDefaultChannel string `json:"server_default_channel" yaml:"server_default_channel"`
}
