package signature_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/signature"
	"github.com/FuturFusion/operations-center/internal/signature/signaturetest"
)

func TestVerifierVerify(t *testing.T) {
	payload := []byte(`This is some random text`)

	caCert, cert, key := signaturetest.GenerateCertChain(t)

	signedContent := signaturetest.SignContent(t, cert, key, payload)

	v := signature.NewVerifier(caCert)
	content, err := v.Verify(signedContent)
	require.NoError(t, err)
	require.Equal(t, string(payload), string(content))
}

func TestVerifierVerifyFileWithDefaultKey(t *testing.T) {
	v := signature.NewVerifier(nil)
	_, err := v.VerifyFile("testdata/index.sjson")
	require.NoError(t, err)
}

func TestNoopVerifier(t *testing.T) {
	payload := []byte(`This is some random text`)

	_, cert, key := signaturetest.GenerateCertChain(t)

	signedContent := signaturetest.SignContent(t, cert, key, payload)

	tmpDir := t.TempDir()
	sjsonFilename := filepath.Join(tmpDir, "test.sjson")
	err := os.WriteFile(sjsonFilename, signedContent, 0o600)
	require.NoError(t, err)

	v := signature.NewNoopVerifier()

	content, err := v.VerifyFile(sjsonFilename)
	require.NoError(t, err)
	require.Equal(t, string(payload), string(content))
}
