package signaturetest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func SignContent(t *testing.T, cert []byte, key []byte, payload []byte) []byte {
	t.Helper()

	tmpDir := t.TempDir()

	signCertFilename := filepath.Join(tmpDir, "sign.crt")
	signKeyFilename := filepath.Join(tmpDir, "sign.key")

	err := os.WriteFile(signCertFilename, cert, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(signKeyFilename, key, 0o600)
	require.NoError(t, err)

	stdoutBuf := bytes.Buffer{}

	cmd := exec.Command("openssl", "smime", "-sign", "-text", "-inkey", signKeyFilename, "-signer", signCertFilename)
	cmd.Stdin = bytes.NewBuffer(payload)
	cmd.Stdout = &stdoutBuf
	err = cmd.Run()
	require.NoError(t, err)

	return stdoutBuf.Bytes()
}
