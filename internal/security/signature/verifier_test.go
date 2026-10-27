package signature_test

import (
	"os"
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
