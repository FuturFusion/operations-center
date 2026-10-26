package system

import "fmt"

// NetworkFull represents the network config, the part of this member and the part shared by all cluster members.
//
// swagger:model
type NetworkFull struct {
	Network       `yaml:",inline"`
	NetworkGlobal `yaml:",inline"`
}

// SettingsFull represents the system settings, the part of this member and the part shared by all cluster members.
//
// swagger:model
type SettingsFull struct {
	Settings       `yaml:",inline"`
	SettingsGlobal `yaml:",inline"`
}

// NetworkFullPut represents the user-configurable network config, the part of this member and the part shared by all cluster members.
//
// swagger:model
type NetworkFullPut struct {
	NetworkPut       `yaml:",inline"`
	NetworkGlobalPut `yaml:",inline"`
}

// SettingsFullPut represents the user-configurable system settings, the part of this member and the part shared by all cluster members.
//
// swagger:model
type SettingsFullPut struct {
	SettingsPut       `yaml:",inline"`
	SettingsGlobalPut `yaml:",inline"`
}

// SharedConfig represents the shared cluster-wide config.
type SharedConfig struct {
	// Network configuration shared by all cluster members.
	Network NetworkGlobal `json:"network" yaml:"network"`

	// Security configuration shared by all cluster members.
	Security Security `json:"security" yaml:"security"`

	// System settings configuration shared by all cluster members.
	Settings SettingsGlobal `json:"settings" yaml:"settings"`

	// Updates configuration shared by all cluster members.
	Updates Updates `json:"updates" yaml:"updates"`

	// Cluster configuration shared by all cluster members.
	Cluster Cluster `json:"cluster" yaml:"cluster"`
}

// NetworkGlobal represents the system's network configuration.
//
// swagger:model
type NetworkGlobal struct {
	NetworkGlobalPut `yaml:",inline"`
}

// NetworkGlobalPut represents the fields available for an update of the
// system's network configuration.
//
// swagger:model
type NetworkGlobalPut struct {
	// Address of Operations Center which is used by managed servers to connect.
	OperationsCenterAddress string `json:"address" yaml:"address"`
}

// SettingsGlobal represents global system settings.
//
// swagger:model
type SettingsGlobal struct {
	SettingsGlobalPut `yaml:",inline"`
}

// SettingsGlobalPut represents the fields available for an update of the global
// system settings.
//
// swagger:model
type SettingsGlobalPut struct {
	// ServerRegistrationScriptlet hold the server registration scriptlet.
	ServerRegistrationScriptlet string `json:"server_registration_scriptlet" yaml:"server_registration_scriptlet"`
}

// Cluster represents shared cluster membership configuration.
//
// swagger:model SystemCluster
type Cluster struct {
	ClusterPut `yaml:",inline"`
}

// ClusterPut represents user configurable cluster membership configuration.
//
// swagger:model SystemClusterPut
type ClusterPut struct {
	OfflineThresholdSeconds int `json:"offline_threshold_seconds" yaml:"offline_threshold_seconds"`
}

// Security represents the system's security configuration.
//
// swagger:model
type Security struct {
	SecurityPut `yaml:",inline"`
}

// SecurityPut represents the fields available for an update of the
// system's security configuration.
//
// swagger:model
type SecurityPut struct {
	// OIDC configuration.
	OIDC SecurityOIDC `json:"oidc" yaml:"oidc"`

	// OpenFGA configuration.
	OpenFGA SecurityOpenFGA `json:"openfga" yaml:"openfga"`

	// ACME configuration.
	ACME SecurityACME `json:"acme" yaml:"acme"`

	// An array of SHA256 certificate fingerprints that belong to trusted TLS clients.
	TrustedTLSClientCertFingerprints []string `json:"trusted_tls_client_cert_fingerprints" yaml:"trusted_tls_client_cert_fingerprints"`

	// An array of X509 PEM encoded certificates that belong to trusted TLS clients.
	// The SHA256 fingerprint of each certificate is derived when the configuration
	// is loaded and trusted in addition to TrustedTLSClientCertFingerprints.
	// In contrast to bare fingerprints, these certificates are
	// also passed on to the servers and clusters deployed by Operations Center, if
	// the user did not provide their own set.
	TrustedTLSClientCertificates []string `json:"trusted_tls_client_certificates" yaml:"trusted_tls_client_certificates"`

	// An array of trusted HTTPS proxy addresses.
	TrustedHTTPSProxies []string `json:"trusted_https_proxies" yaml:"trusted_https_proxies"`
}

// SecurityOIDC is the OIDC related part of the system's security
// configuration.
type SecurityOIDC struct {
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

// SecurityOpenFGA is the OpenFGA related part of the system's security
// configuration.
type SecurityOpenFGA struct {
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

type SecurityACME struct {
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

// Updates represents the system's updates configuration.
//
// swagger:model
type Updates struct {
	UpdatesPut `yaml:",inline"`
}

// UpdatesPut represents the fields available for an update of the
// system's updates configuration.
//
// swagger:model
type UpdatesPut struct {
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

	// ImageServerAuthenticationByQueryParam is a flag that allows to switch from
	// HTTP header based image server authentication to query parameter instead.
	// If set to true, authentication is done by `token` query parameter on the
	// first request, if set to false, authentication is done by HTTP header.
	ImageServerAuthenticationByQueryParam bool `json:"image_server_authentication_by_query_param" yaml:"image_server_authentication_by_query_param"`
}
