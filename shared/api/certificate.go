package api

import (
	"crypto/x509"
	"encoding/json"

	"github.com/FuturFusion/operations-center/internal/util/certificate"
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
		parsedCert.Certificate, err = certificate.Decode([]byte(certStr))
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
		parsedCert.Certificate, err = certificate.Decode([]byte(certStr))
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
	return certificate.X509EncodeToPEM(c.Certificate)
}
