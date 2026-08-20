// Package tls provides helpers to work with the X509 client certificates
// trusted by Operations Center.
package tls

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	incusapi "github.com/lxc/incus/v7/shared/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
)

// trustedClientCertificateNamePrefix is prepended to the name of the Incus
// certificates derived from the trusted client certificates of Operations
// Center. It marks them as managed by Operations Center.
const trustedClientCertificateNamePrefix = "oc-trusted-"

// ParseCertificate parses a X509 PEM encoded certificate.
func ParseCertificate(certificatePEM string) (*x509.Certificate, error) {
	pemBlock, _ := pem.Decode([]byte(certificatePEM))
	if pemBlock == nil {
		return nil, errors.New("Certificate is not a valid PEM block")
	}

	return x509.ParseCertificate(pemBlock.Bytes)
}

// CertificateFingerprints returns the canonical SHA256 fingerprints of the given
// X509 PEM encoded certificates, which parse successfully, in the order the
// certificates are provided and without duplicates.
func CertificateFingerprints(certificatesPEM []string) ([]string, error) {
	if len(certificatesPEM) == 0 {
		return nil, nil
	}

	var errs []error

	fingerprints := make([]string, 0, len(certificatesPEM))
	seenFingerprints := make(map[string]struct{}, len(certificatesPEM))

	for i, certificatePEM := range certificatesPEM {
		cert, err := ParseCertificate(certificatePEM)
		if err != nil {
			errs = append(errs, fmt.Errorf("Certificate at position %d: %w", i+1, err))
			continue
		}

		fingerprint := incustls.CertFingerprint(cert)

		_, ok := seenFingerprints[fingerprint]
		if ok {
			continue
		}

		seenFingerprints[fingerprint] = struct{}{}
		fingerprints = append(fingerprints, fingerprint)
	}

	return fingerprints, errors.Join(errs...)
}

// FilterCertificatesByFingerprints returns those of the given X509 PEM encoded
// certificates, whose fingerprint is part of the given list of fingerprints, if
// matching is true, respectively those, whose fingerprint is not part of the
// list, if matching is false.
func FilterCertificatesByFingerprints(certificatesPEM []string, fingerprints []string, matching bool) ([]string, error) {
	if len(certificatesPEM) == 0 {
		return nil, nil
	}

	knownFingerprints := make(map[string]struct{}, len(fingerprints))
	for _, fingerprint := range fingerprints {
		knownFingerprints[fingerprint] = struct{}{}
	}

	filteredCertificates := make([]string, 0, len(certificatesPEM))

	for _, certificatePEM := range certificatesPEM {
		cert, err := ParseCertificate(certificatePEM)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse certificate: %w", err)
		}

		_, ok := knownFingerprints[incustls.CertFingerprint(cert)]
		if ok != matching {
			continue
		}

		filteredCertificates = append(filteredCertificates, certificatePEM)
	}

	return filteredCertificates, nil
}

type TrustedClientCertificate struct {
	incusapi.CertificatesPost

	Fingerprint string
}

// TrustedClientCertificates converts the given X509 PEM encoded client
// certificates into Incus certificate definitions.
//
// The name of each certificate is derived from its SHA256 fingerprint.
func TrustedClientCertificates(certificatesPEM []string) ([]incusapi.CertificatesPost, error) {
	trustedCertificates, err := TrustedClientCertificatesWithFingerprint(certificatesPEM)
	if err != nil {
		return nil, err
	}

	if len(trustedCertificates) == 0 {
		return nil, nil
	}

	certificates := make([]incusapi.CertificatesPost, 0, len(trustedCertificates))
	for _, trustedCertificate := range trustedCertificates {
		certificates = append(certificates, trustedCertificate.CertificatesPost)
	}

	return certificates, nil
}

// TrustedClientCertificatesWithFingerprint converts the given X509 PEM encoded
// client certificates into Incus certificate definitions, each together with
// the SHA256 fingerprint of the respective certificate.
//
// The name of each certificate is derived from its SHA256 fingerprint.
func TrustedClientCertificatesWithFingerprint(certificatesPEM []string) ([]TrustedClientCertificate, error) {
	if len(certificatesPEM) == 0 {
		return nil, nil
	}

	certificates := make([]TrustedClientCertificate, 0, len(certificatesPEM))
	seenFingerprints := make(map[string]struct{}, len(certificatesPEM))

	for _, certificatePEM := range certificatesPEM {
		cert, err := ParseCertificate(certificatePEM)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse trusted client certificate: %w", err)
		}

		fingerprint := incustls.CertFingerprint(cert)

		// Ensure unique fingerprints.
		_, ok := seenFingerprints[fingerprint]
		if ok {
			continue
		}

		seenFingerprints[fingerprint] = struct{}{}

		certificates = append(certificates, TrustedClientCertificate{
			CertificatesPost: incusapi.CertificatesPost{
				CertificatePut: incusapi.CertificatePut{
					Name:        trustedClientCertificateNamePrefix + fingerprint[:12],
					Description: "Client trusted by Operations Center",
					Type:        "client",
					Restricted:  false,
					Projects:    []string{},
					Certificate: certificatePEM,
				},
			},
			Fingerprint: fingerprint,
		})
	}

	return certificates, nil
}
