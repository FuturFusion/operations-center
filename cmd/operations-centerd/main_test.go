package main

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMain0Version(t *testing.T) {
	stdoutBuf := bytes.Buffer{}

	err := main0([]string{"--version"}, &stdoutBuf, nil, mockEnv{})
	require.NoError(t, err)

	require.Equal(t, "0.0.1\n", stdoutBuf.String())
}

func TestMain0RunDaemon(t *testing.T) {
	var daemonErr error
	logs := make(chan string, 10)
	stderrWriter := chanWriter{
		c: logs,
	}

	tmpDir := t.TempDir()

	// Start daemon.
	go func() {
		daemonErr = main0([]string{"--verbose"}, nil, stderrWriter, mockEnv{
			logDir:     tmpDir,
			varDir:     tmpDir,
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
		})
	}()

	waitFor(t, logs, "Daemon started", 5000*time.Millisecond)

	// Check for errors during daemon start (require must only be used in the main test go routing)
	require.NoError(t, daemonErr)

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Get("https://localhost:7443")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.JSONEq(t, `{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":["/1.0"]}`, string(body))

	// Shutdown daemon with interrupt signal.
	p, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)

	err = p.Signal(os.Interrupt)
	require.NoError(t, err)

	// Wait for shutdown to complete.
	waitFor(t, logs, "Daemon shutdown completed successfully", 5000*time.Millisecond)
}

func TestMain0RunDaemonStartError(t *testing.T) {
	tmpDir := t.TempDir()

	// Write invalid config.yml.
	err := os.Mkdir(filepath.Join(tmpDir, "invalid"), 0o770)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "invalid/config.yml"), []byte(`{`), 0o660)
	require.NoError(t, err)

	tests := []struct {
		name       string
		args       []string
		logDir     string
		varDir     string
		unixSocket string

		wantErrContains string
	}{
		{
			name: "invalid command",

			args:   []string{"foo"},
			logDir: tmpDir,

			wantErrContains: `Unknown command "foo"`,
		},
		{
			name: "invalid log directory",

			args:       []string{""},
			logDir:     filepath.Join(tmpDir, "inexisting"), // this directory does not exist.
			varDir:     tmpDir,
			unixSocket: filepath.Join(tmpDir, "unix.socket"),

			wantErrContains: "no such file or directory",
		},
		{
			name: "invalid var directory",

			args:       []string{""},
			logDir:     tmpDir,
			varDir:     filepath.Join(tmpDir, "invalid"),
			unixSocket: filepath.Join(tmpDir, "unix.socket"),

			wantErrContains: "Failed to load config from",
		},
		{
			name: "invalid unix socket",

			args:       []string{""},
			logDir:     tmpDir,
			varDir:     tmpDir,
			unixSocket: tmpDir, // invalid for unix socket, since it is a directory.

			wantErrContains: "Failed to start daemon",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := main0(tc.args, nil, nil, mockEnv{
				logDir:     tc.logDir,
				varDir:     tc.varDir,
				unixSocket: tc.unixSocket,
			})
			require.ErrorContains(t, err, tc.wantErrContains)
		})
	}
}

func waitFor(t *testing.T, in chan string, want string, d time.Duration) {
	t.Helper()

	timer := time.NewTimer(d)
	defer timer.Stop()

	for {
		select {
		case line := <-in:
			t.Log(line)
			if strings.Contains(line, want) {
				return
			}

		case <-timer.C:
			t.Fatalf("Timeout %v expired while waiting for %s", d, want)
		}
	}
}

type mockEnv struct {
	logDir     string
	varDir     string
	unixSocket string
}

func (e mockEnv) LogDir() string        { return e.logDir }
func (e mockEnv) VarDir() string        { return e.varDir }
func (e mockEnv) GetUnixSocket() string { return e.unixSocket }

type chanWriter struct {
	c chan string
}

func (c chanWriter) Write(p []byte) (n int, err error) {
	c.c <- strings.TrimRight(string(p), "\n")
	return len(p), nil
}
