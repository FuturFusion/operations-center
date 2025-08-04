package api

// SystemCertificatePut represents the fields available for an update of the
// system certificate (server certificate) and key.
//
// swagger:model
type SystemCertificatePut struct {
	// The new certificate (X509 PEM encoded) for the system (server certificate).
	// Example: X509 PEM certificate
	Certificate string `json:"certificate"`

	// The new certificate key (X509 PEM encoded) for the system (server key).
	// Example: X509 PEM certificate key
	Key string `json:"key"`
}
