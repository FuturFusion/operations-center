package api

const (
	APIVersion string = "1.0"
	APIStatus  string = "devel"
)

// ServerUntrusted represents a server configuration for an untrusted client
//
// swagger:model
type ServerUntrusted struct {
	// Support status of the current API (one of "devel", "stable" or "deprecated")
	// Read only: true
	// Example: stable
	APIStatus string `json:"api_status" yaml:"api_status"`

	// API version number
	// Read only: true
	// Example: 1.0
	APIVersion string `json:"api_version" yaml:"api_version"`

	// Authentication protocol of a trusted client (one of "tls", "oidc" or "unix"), "untrusted" otherwise.
	// A metrics only certificate also reports "tls", even though it can only read the metrics.
	// Read only: true
	// Example: untrusted
	Auth string `json:"auth" yaml:"auth"`

	// List of supported authentication methods
	// Read only: true
	// Example: ["tls"]
	//
	// API extension: macaroon_authentication
	AuthMethods []string `json:"auth_methods" yaml:"auth_methods"`

	// Server version number
	// Read only: true
	// Example: 1.0.0
	ServerVersion string `json:"server_version" yaml:"server_version"`
}
