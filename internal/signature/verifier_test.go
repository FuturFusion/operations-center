package signature_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/signature"
)

func TestVerifier(t *testing.T) {
	tests := []struct {
		name                string
		publicKeyMarshaller func(t *testing.T, publicKey any, privKey any) []byte

		assertNewVerifierErr require.ErrorAssertionFunc
		assertVerifyErr      require.ErrorAssertionFunc
	}{
		{
			name: "success - CERTIFICATE",
			publicKeyMarshaller: func(t *testing.T, publicKey any, privKey any) []byte {
				t.Helper()

				ca := &x509.Certificate{
					SerialNumber: big.NewInt(1),
				}

				caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, publicKey, privKey)
				require.NoError(t, err)

				return pem.EncodeToMemory(&pem.Block{
					Type:  "CERTIFICATE",
					Bytes: caBytes,
				})
			},

			assertNewVerifierErr: require.NoError,
			assertVerifyErr:      require.NoError,
		},
		{
			name: "success - PUBLIC KEY",
			publicKeyMarshaller: func(t *testing.T, publicKey any, _ any) []byte {
				t.Helper()
				pkixKey, err := x509.MarshalPKIXPublicKey(publicKey)
				require.NoError(t, err)
				return pem.EncodeToMemory(&pem.Block{
					Type:  "PUBLIC KEY",
					Bytes: pkixKey,
				})
			},

			assertNewVerifierErr: require.NoError,
			assertVerifyErr:      require.NoError,
		},
		{
			name: "success - RSA PUBLIC KEY",
			publicKeyMarshaller: func(t *testing.T, publicKey any, _ any) []byte {
				t.Helper()
				rsaKey, ok := publicKey.(*rsa.PublicKey)
				require.True(t, ok)
				return pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PUBLIC KEY",
					Bytes: x509.MarshalPKCS1PublicKey(rsaKey),
				})
			},

			assertNewVerifierErr: require.NoError,
			assertVerifyErr:      require.NoError,
		},

		{
			name: "error - invalid PEM",
			publicKeyMarshaller: func(t *testing.T, publicKey any, _ any) []byte {
				t.Helper()
				return []byte(`invalid`)
			},

			assertNewVerifierErr: require.Error,
			assertVerifyErr:      require.Error,
		},
		{
			name: "error - invalid CERTIFICATE",
			publicKeyMarshaller: func(t *testing.T, publicKey any, privKey any) []byte {
				t.Helper()

				return pem.EncodeToMemory(&pem.Block{
					Type:  "CERTIFICATE",
					Bytes: []byte(`invalid`),
				})
			},

			assertNewVerifierErr: require.Error,
			assertVerifyErr:      require.Error,
		},
		{
			name: "error - invalid PUBLIC KEY",
			publicKeyMarshaller: func(t *testing.T, publicKey any, privKey any) []byte {
				t.Helper()

				return pem.EncodeToMemory(&pem.Block{
					Type:  "PUBLIC KEY",
					Bytes: []byte(`invalid`),
				})
			},

			assertNewVerifierErr: require.Error,
			assertVerifyErr:      require.Error,
		},
		{
			name: "error - invalid RSA PUBLIC KEY",
			publicKeyMarshaller: func(t *testing.T, publicKey any, privKey any) []byte {
				t.Helper()

				return pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PUBLIC KEY",
					Bytes: []byte(`invalid`),
				})
			},

			assertNewVerifierErr: require.Error,
			assertVerifyErr:      require.Error,
		},
		{
			name: "error - unsupported public key type",
			publicKeyMarshaller: func(t *testing.T, publicKey any, privKey any) []byte {
				t.Helper()

				return pem.EncodeToMemory(&pem.Block{
					Type:  "INVALID",
					Bytes: []byte(`invalid`),
				})
			},

			assertNewVerifierErr: require.Error,
			assertVerifyErr:      require.Error,
		},
		{
			name: "error - invalid signature",
			publicKeyMarshaller: func(t *testing.T, publicKey any, _ any) []byte {
				t.Helper()
				rsaKey, ok := publicKey.(*rsa.PublicKey)
				require.True(t, ok)
				rsaKeyCopy := &rsa.PublicKey{
					N: rsaKey.N,
					E: rsaKey.E + 1, // change exponent to make the public key not fit the signature
				}

				return pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PUBLIC KEY",
					Bytes: x509.MarshalPKCS1PublicKey(rsaKeyCopy),
				})
			},

			assertNewVerifierErr: require.NoError,
			assertVerifyErr:      require.Error,
		},
	}

	payload := []byte(`This is some random text`)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			privKey, pubKeyPEM := generateKeys(t, tc.publicKeyMarshaller)

			hash := sha256.Sum256(payload)

			signatureRaw, err := rsa.SignPSS(rand.Reader, privKey, crypto.SHA256, hash[:], nil)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(tmpDir, "file.txt"), payload, 0o600)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(tmpDir, "file.txt.sig"), []byte(hex.EncodeToString(signatureRaw)), 0o600)
			require.NoError(t, err)

			verifier, err := signature.NewVerifier(pubKeyPEM)
			tc.assertNewVerifierErr(t, err)

			err = verifier.VerifyFile(filepath.Join(tmpDir, "file.txt"))
			tc.assertVerifyErr(t, err)
		})
	}
}

func generateKeys(t *testing.T, publicKeyMarshaller func(t *testing.T, publicKey any, privKey any) []byte) (privKey *rsa.PrivateKey, pubKeyPEM []byte) {
	t.Helper()

	var err error

	privKey, err = rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	publicKey := &privKey.PublicKey

	pubKeyPEM = publicKeyMarshaller(t, publicKey, privKey)

	return privKey, pubKeyPEM
}

func TestNoopVerifier(t *testing.T) {
	v := signature.NewNoopVerifier()

	err := v.Verify([]byte{}, []byte{})
	require.NoError(t, err)

	err = v.VerifyFile("foobar")
	require.NoError(t, err)
}
