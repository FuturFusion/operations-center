package tls

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

const (
	// ClientCertificateFilename is the name of the client certificate in the var dir.
	ClientCertificateFilename = "client.crt"

	// ClientKeyFilename is the name of the client key in the var dir.
	ClientKeyFilename = "client.key"
)

// ClientKeypair is the keypair Operations Center connects to the servers it manages with.
type ClientKeypair struct {
	Certificate string
	Key         string

	// Fingerprint of the certificate, empty if it is not valid.
	Fingerprint string
}

// ClientKeypairHolder holds the client keypair, which can be replaced at runtime.
type ClientKeypairHolder struct {
	keypair atomic.Pointer[ClientKeypair]
}

// NewClientKeypairHolder returns a holder of the given client keypair.
func NewClientKeypairHolder(certificatePEM string, keyPEM string) *ClientKeypairHolder {
	h := &ClientKeypairHolder{}
	h.Set(certificatePEM, keyPEM)

	return h
}

// Get returns the current client keypair.
func (h *ClientKeypairHolder) Get() ClientKeypair {
	return *h.keypair.Load()
}

// Set replaces the client keypair.
func (h *ClientKeypairHolder) Set(certificatePEM string, keyPEM string) {
	fingerprint, err := incustls.CertFingerprintStr(certificatePEM)
	if err != nil {
		fingerprint = ""
	}

	h.keypair.Store(&ClientKeypair{
		Certificate: certificatePEM,
		Key:         keyPEM,
		Fingerprint: fingerprint,
	})
}

// LoadClientKeypair reads the client keypair from varDir.
func LoadClientKeypair(varDir string) (certificatePEM string, keyPEM string, _ error) {
	certificate, err := os.ReadFile(filepath.Join(varDir, ClientCertificateFilename))
	if err != nil {
		return "", "", fmt.Errorf("Failed to read client certificate: %w", err)
	}

	key, err := os.ReadFile(filepath.Join(varDir, ClientKeyFilename))
	if err != nil {
		return "", "", fmt.Errorf("Failed to read client key: %w", err)
	}

	return string(certificate), string(key), nil
}

// WriteClientKeypair replaces the client keypair in varDir.
func WriteClientKeypair(varDir string, certificatePEM string, keyPEM string) error {
	err := file.WriteFileAtomic(filepath.Join(varDir, ClientKeyFilename), []byte(keyPEM), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write client key: %w", err)
	}

	err = file.WriteFileAtomic(filepath.Join(varDir, ClientCertificateFilename), []byte(certificatePEM), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write client certificate: %w", err)
	}

	return nil
}

// GenerateClientKeypair replaces the client keypair in varDir with a newly generated one.
func GenerateClientKeypair(varDir string) (certificatePEM string, keyPEM string, _ error) {
	certificate, key, err := incustls.GenerateMemCert(true, false)
	if err != nil {
		return "", "", fmt.Errorf("Failed to generate client certificate: %w", err)
	}

	err = WriteClientKeypair(varDir, string(certificate), string(key))
	if err != nil {
		return "", "", err
	}

	return string(certificate), string(key), nil
}
