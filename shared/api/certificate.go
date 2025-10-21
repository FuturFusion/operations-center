package api

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
)

type Certificate struct {
	*x509.Certificate
}

func (c *Certificate) UnmarshalYAML(unmarshal func(v any) error) error {
	var certStr string
	err := unmarshal(&certStr)
	if err != nil {
		return err
	}

	parsedCert := Certificate{}
	if certStr != "" {
		parsedCert.Certificate, err = decodeCert([]byte(certStr))
		if err != nil {
			return err
		}
	}

	*c = parsedCert

	return nil
}

func (c Certificate) MarshalYAML() (any, error) {
	return c.String(), nil
}

func (c *Certificate) UnmarshalJSON(b []byte) error {
	var certStr string
	err := json.Unmarshal(b, &certStr)
	if err != nil {
		return err
	}

	parsedCert := Certificate{}
	if certStr != "" {
		parsedCert.Certificate, err = decodeCert([]byte(certStr))
		if err != nil {
			return err
		}
	}

	*c = parsedCert

	return nil
}

func (c Certificate) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c Certificate) String() string {
	return X509CertEncodeToPEM(c.Certificate)
}

func X509CertEncodeToPEM(cert *x509.Certificate) string {
	if cert != nil {
		return CertEncodeToPEM(cert.Raw)
	}

	return ""
}

func CertEncodeToPEM(rawCert []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rawCert}))
}

func decodeCert(certBytes []byte) (*x509.Certificate, error) {
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
