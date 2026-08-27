package certificate

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"
)

// Decode decodes a PEM encoded X509 certificate.
func Decode(certBytes []byte) (*x509.Certificate, error) {
	certBlock, _ := pem.Decode(certBytes)
	if certBlock == nil {
		return nil, fmt.Errorf("Certificate must be base64 encoded PEM certificate")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse x509 certificate: %w", err)
	}

	return cert, nil
}

// EncodeToPEM encodes a raw DER certificate in PEM format.
func EncodeToPEM(rawCert []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rawCert}))
}

// X509EncodeToPEM encodes an X509 certificate in PEM format.
func X509EncodeToPEM(cert *x509.Certificate) string {
	if cert != nil {
		return EncodeToPEM(cert.Raw)
	}

	return ""
}

// Validate reports whether the PEM encoded certificate is currently valid.
// A true return together with a non nil error means, the certificate is valid,
// but expires within expiryThreshold.
func Validate(certPEM string, expiryThreshold time.Duration) (valid bool, _ error) {
	cert, err := Decode([]byte(certPEM))
	if err != nil {
		return false, err
	}

	if time.Now().Before(cert.NotBefore) {
		return false, fmt.Errorf("The provided certificate isn't valid yet")
	}

	if time.Now().After(cert.NotAfter) {
		return false, fmt.Errorf("The provided certificate is expired")
	}

	if time.Now().Add(expiryThreshold).After(cert.NotAfter) {
		return true, fmt.Errorf("The provided certificate expires within %d days, expiration date: %s", int(expiryThreshold.Hours()/24), cert.NotAfter.String())
	}

	return true, nil
}
