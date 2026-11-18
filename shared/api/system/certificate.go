package system

import (
	"time"
)

// Certificate represents the system's certificate (server certificate).
// The corresponding private key is never part of this representation.
//
// swagger:model
type Certificate struct {
	// The certificate (X509 PEM encoded) of the system (server certificate).
	// Example:
	//	-----BEGIN CERTIFICATE-----
	//	...
	//	-----END CERTIFICATE-----
	Certificate string `json:"certificate" yaml:"certificate"`

	// Fingerprint in SHA256 format of the certificate.
	// Example: fd200419b271f1dc2a5591b693cc5774b7f234e1ff8c6b78ad703b6888fe2b69
	Fingerprint string `json:"fingerprint" yaml:"fingerprint"`

	// Subject is the name of the subject of the certificate.
	// Example: CN=operations-center.example.com,O=Example Inc.
	Subject string `json:"subject" yaml:"subject"`

	// Issuer is the name of the issuer of the certificate.
	// Example: CN=Example CA,O=Example Inc.
	Issuer string `json:"issuer" yaml:"issuer"`

	// NotBefore is the start of the validity period of the certificate in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	NotBefore time.Time `json:"not_before" yaml:"not_before"`

	// NotAfter is the end of the validity period of the certificate in RFC3339 format.
	// Example: 2025-11-12T16:15:00Z
	NotAfter time.Time `json:"not_after" yaml:"not_after"`

	// DNSNames holds the DNS subject alternative names of the certificate.
	// Example: ["operations-center.example.com"]
	DNSNames []string `json:"dns_names" yaml:"dns_names"`

	// IPAddresses holds the IP subject alternative names of the certificate.
	// Example: ["127.0.0.1", "::1"]
	IPAddresses []string `json:"ip_addresses" yaml:"ip_addresses"`
}

// CertificatePost represents the fields available for an update of the
// system certificate (server certificate) and key.
//
// swagger:model
type CertificatePost struct {
	// The new certificate (X509 PEM encoded) for the system (server certificate).
	// Optionally, intermediate CA certificates are appended to the server
	// certificate to form a certificate chain. The server certificate is
	// expected first, followed by the intermediate CA certificates in order.
	// Example: X509 PEM certificate
	Certificate string `json:"certificate" yaml:"certificate"`

	// The new certificate key (X509 PEM encoded) for the system (server key).
	// Example: X509 PEM certificate key
	Key string `json:"key" yaml:"key"`
}
