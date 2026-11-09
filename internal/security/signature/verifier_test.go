package signature_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/signature"
	"github.com/FuturFusion/operations-center/internal/security/signature/signaturetest"
)

func TestVerifierVerify(t *testing.T) {
	t.Parallel()

	payload := []byte(`This is some random text`)

	caCert, cert, key := signaturetest.GenerateCertChain(t)

	signedContent := signaturetest.SignContent(t, cert, key, payload)

	v := signature.NewVerifier(caCert)
	content, err := v.Verify(signedContent)
	require.NoError(t, err)
	require.Equal(t, string(payload), string(content))
}

func TestVerifierVerifyWithDefaultKey(t *testing.T) {
	t.Parallel()

	signedContent, err := os.ReadFile("testdata/index.sjson")
	require.NoError(t, err)

	v := signature.NewVerifier(nil)
	_, err = v.Verify(signedContent)
	require.NoError(t, err)
}

func TestVerifierVerifyIgnoresSystemTrustStore(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func(t *testing.T, rogueCAFile string)
	}{
		{
			name: "default CA directory",
			setupEnv: func(t *testing.T, rogueCAFile string) {
				t.Helper()

				certDir := t.TempDir()
				rogueCA, err := os.ReadFile(rogueCAFile)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(certDir, subjectHash(t, rogueCAFile)+".0"), rogueCA, 0o600)
				require.NoError(t, err)

				t.Setenv("SSL_CERT_DIR", certDir)
			},
		},
		{
			name: "default CA file",
			setupEnv: func(t *testing.T, rogueCAFile string) {
				t.Helper()

				t.Setenv("SSL_CERT_FILE", rogueCAFile)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			rogueCACert, rogueCert, rogueKey := signaturetest.GenerateCertChain(t)

			rogueCAFile := filepath.Join(t.TempDir(), "rogue-ca.crt")
			err := os.WriteFile(rogueCAFile, rogueCACert, 0o600)
			require.NoError(t, err)

			tc.setupEnv(t, rogueCAFile)

			signedContent := signaturetest.SignContent(t, rogueCert, rogueKey, []byte(`This is some random text`))

			// Run test
			// Pin the default root CA. Its subject differs from the rogue CA, so openssl looks for the rogue CA in the system trust store.
			content, err := signature.NewVerifier(nil).Verify(signedContent)

			// Assert
			require.Error(t, err, "content signed by a CA from the system trust store must not pass verification")
			require.Empty(t, content, "verification failure must not return content")
		})
	}
}

func subjectHash(t *testing.T, certFile string) string {
	t.Helper()

	out, err := exec.Command("openssl", "x509", "-hash", "-noout", "-in", certFile).Output()
	require.NoError(t, err)

	return strings.TrimSpace(string(out))
}

func TestNoopVerifier(t *testing.T) {
	t.Parallel()

	payload := []byte(`This is some random text`)

	_, cert, key := signaturetest.GenerateCertChain(t)

	signedContent := signaturetest.SignContent(t, cert, key, payload)

	v := signature.NewNoopVerifier()

	content, err := v.Verify(signedContent)
	require.NoError(t, err)
	require.Equal(t, string(payload), string(content))
}
