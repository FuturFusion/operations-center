package api

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
	OIDC    SystemSecurityOIDC    `json:"oidc" yaml:"oidc"`
	OpenFGA SystemSecurityOpenFGA `json:"openfga" yaml:"openfga"`

	// An array of SHA256 certificate fingerprints that belong to trusted TLS clients.
	TrustedTLSClientCertFingerprints []string `json:"trusted_tls_client_cert_fingerprints" yaml:"trusted_tls_client_cert_fingerprints"`
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
	// Empty filter expression does not filter at all.
	// Example: 'stable' in Channels
	FilterExpression string `json:"filter_expression" yaml:"filter_expression"`

	// Filter expression for update files using https://expr-lang.org/ on struct
	// provisioning.UpdateFile.
	// If a filter is defined, the filter needs to evaluate to true for the file
	// being fetched by Operations Center.
	// Empty filter expression does not filter at all.
	//
	// For file filter expression, the following helper functions are available:
	//   - AppliesToArchitecture(arch string) bool
	//       Returns true if the Architecture field matches the given arch string
	//       or if the Architecture field is not set.
	//
	// Examples:
	//   AppliesToArchitecture("x86_64")
	//   Architecture == "x86_64"
	FileFilterExpression string `json:"file_filter_expression" yaml:"file_filter_expression"`
}
