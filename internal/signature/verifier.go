package signature

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

type Verifier interface {
	Verify(sjson []byte) (content []byte, _ error)
	VerifyFile(filename string) (content []byte, _ error)
}

type verifier struct {
	rootCAPEM []byte
}

func NewVerifier(rootCAPEM []byte) Verifier {
	if len(rootCAPEM) == 0 {
		rootCAPEM = []byte(defaultRootCA)
	}

	return &verifier{
		rootCAPEM: rootCAPEM,
	}
}

func (v verifier) Verify(sjson []byte) ([]byte, error) {
	rootCAFile, err := os.CreateTemp("", "operations-center-updates-rootca-*.crt")
	if err != nil {
		return nil, fmt.Errorf("Failed to create temporary file for root CA PEM: %w", err)
	}

	defer func() {
		_ = rootCAFile.Close()
		_ = os.Remove(rootCAFile.Name())
	}()

	_, err = rootCAFile.Write(v.rootCAPEM)
	if err != nil {
		return nil, fmt.Errorf("Failed to write root CA PEM to temporary file: %w", err)
	}

	err = rootCAFile.Close()
	if err != nil {
		return nil, fmt.Errorf("Failed to close root CA PEM temporary file: %w", err)
	}

	stdoutBuf := bytes.Buffer{}
	stderrBuf := bytes.Buffer{}

	cmd := exec.Command("openssl", "smime", "-text", "-verify", "-CAfile", rootCAFile.Name())
	cmd.Stdin = bytes.NewBuffer(sjson)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf(`Failed to verify signature using "openssl" error output: %q, error: %w`, stderrBuf.String(), err)
	}

	return stdoutBuf.Bytes(), nil
}

func (v verifier) VerifyFile(filename string) ([]byte, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("Unable to read %q: %w", filename, err)
	}

	return v.Verify(content)
}

type noopVerifier struct{}

func NewNoopVerifier() Verifier {
	return noopVerifier{}
}

func (noopVerifier) Verify(sjson []byte) ([]byte, error) {
	stdoutBuf := bytes.Buffer{}

	cmd := exec.Command("openssl", "smime", "-text", "-verify", "-noverify")
	cmd.Stdin = bytes.NewBuffer(sjson)
	cmd.Stdout = &stdoutBuf
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf(`Failed to process smime in noop verifier: %w`, err)
	}

	return stdoutBuf.Bytes(), nil
}

func (n noopVerifier) VerifyFile(filename string) ([]byte, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("Unable to read %q: %w", filename, err)
	}

	return n.Verify(content)
}
