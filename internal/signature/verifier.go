package signature

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

type Verifier interface {
	Verify(content []byte, signature []byte) error
	VerifyFile(filename string) error
}

type verifier struct {
	verifySignature func(digest []byte, sig []byte) bool
}

func NewVerifier(pemBody []byte) (Verifier, error) {
	pemBlock, _ := pem.Decode(pemBody)
	if pemBlock == nil {
		return verifier{}, fmt.Errorf("No valid PEM block found")
	}

	var key any

	switch pemBlock.Type {
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(pemBlock.Bytes)
		if err != nil {
			return verifier{}, fmt.Errorf("Failed to process update.signature_verification_pem: %w", err)
		}

		key = cert.PublicKey

	case "PUBLIC KEY":
		publicKey, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
		if err != nil {
			return verifier{}, fmt.Errorf("Failed to parse public key: %s", err)
		}

		key = publicKey

	case "RSA PUBLIC KEY":
		publicKey, err := x509.ParsePKCS1PublicKey(pemBlock.Bytes)
		if err != nil {
			return verifier{}, fmt.Errorf("Failed to parse public key: %s", err)
		}

		key = publicKey

	default:
		return verifier{}, fmt.Errorf("Type %q for pem block not supported", pemBlock.Type)
	}

	var verifySignature func(digest []byte, sig []byte) bool
	switch publicKey := key.(type) {
	case *ecdsa.PublicKey:
		verifySignature = func(hash []byte, sig []byte) bool {
			return ecdsa.VerifyASN1(publicKey, hash, sig)
		}

	case *rsa.PublicKey:
		verifySignature = func(hash []byte, sig []byte) bool {
			err := rsa.VerifyPSS(publicKey, crypto.SHA256, hash, sig, nil)

			return err == nil
		}

	default:
		return verifier{}, fmt.Errorf("Unsupported public key %T", key)
	}

	return verifier{
		verifySignature: verifySignature,
	}, nil
}

func (v verifier) Verify(content []byte, signature []byte) error {
	if v.verifySignature == nil {
		return fmt.Errorf("Verifier is not properly initialized")
	}

	hash := sha256.Sum256(content)

	signatureDecoded, err := hex.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil {
		return fmt.Errorf("Failed to decode signature: %w", err)
	}

	ok := v.verifySignature(hash[:], signatureDecoded)
	if !ok {
		return fmt.Errorf(`Invalid signature for "update.json"`)
	}

	return nil
}

func (v verifier) VerifyFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("Unable to read %q: %w", filename, err)
	}

	signature, err := os.ReadFile(filename + ".sig")
	if err != nil {
		return fmt.Errorf(`Unable to read "%s.sig": %w`, filename, err)
	}

	return v.Verify(content, signature)
}

type noopVerifier struct{}

func NewNoopVerifier() Verifier {
	return noopVerifier{}
}

func (noopVerifier) Verify(_, _ []byte) error {
	return nil
}

func (noopVerifier) VerifyFile(_ string) error {
	return nil
}
