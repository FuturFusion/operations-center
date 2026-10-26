package clustertests

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

// member is an Operations Center daemon run as a separate process.
type member struct {
	binary  string
	name    string
	dir     string
	address string

	cmd    *exec.Cmd
	exited chan struct{}
	client *http.Client
}

func newMember(t *testing.T, binary string, root string, name string, address string) *member {
	t.Helper()

	dir := filepath.Join(root, name)
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)

	socket := filepath.Join(dir, "unix.socket")

	// The path of a unix socket is limited to 108 bytes.
	require.Less(t, len(socket), 100, "The unix socket path %q is too long, set a shorter TMPDIR", socket)

	return &member{
		binary:  binary,
		name:    name,
		dir:     dir,
		address: address,
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

// start starts the daemon and waits until its database is available.
func (m *member) start(t *testing.T) {
	t.Helper()

	// Retry, since a member started right after a leadership handover might fail to update its schema version.
	for range 3 {
		err := m.tryStart()
		if err == nil {
			return
		}

		t.Logf("Failed to start %s, retrying: %v", m.name, err)
		time.Sleep(3 * time.Second)
	}

	t.Fatalf("Failed to start %s", m.name)
}

func (m *member) tryStart() error {
	logFile := filepath.Join(m.dir, "daemon.log")

	offset := int64(0)
	fi, err := os.Stat(logFile)
	if err == nil {
		offset = fi.Size()
	}

	out, err := os.OpenFile(filepath.Join(m.dir, "out.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}

	defer out.Close()

	cmd := exec.Command(m.binary, "--debug", "--logfile", logFile)
	cmd.Env = append(os.Environ(), "OPERATIONS_CENTER_DIR="+m.dir)
	cmd.Stdout = out
	cmd.Stderr = out

	err = cmd.Start()
	if err != nil {
		return err
	}

	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	m.cmd = cmd
	m.exited = exited

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		log := readFrom(logFile, offset)
		if strings.Contains(log, "Cluster database startup completed") {
			return nil
		}

		select {
		case <-exited:
			m.cmd = nil
			return fmt.Errorf("Daemon exited: %s", lastLines(log, 3))

		case <-time.After(500 * time.Millisecond):
		}
	}

	m.kill()

	return errors.New("Timeout waiting for the database")
}

// stop stops the daemon gracefully.
func (m *member) stop() {
	if m.cmd == nil {
		return
	}

	_ = m.cmd.Process.Signal(syscall.SIGINT)

	select {
	case <-m.exited:
		m.cmd = nil

	case <-time.After(30 * time.Second):
		m.kill()
	}
}

// kill kills the daemon without giving it a chance to clean up.
func (m *member) kill() {
	if m.cmd == nil {
		return
	}

	_ = m.cmd.Process.Kill()
	<-m.exited

	m.cmd = nil
}

// clientCertificate returns the client certificate of the member.
func (m *member) clientCertificate(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(m.dir, "client.crt"))
	require.NoError(t, err)

	return string(content)
}

// readFile returns the content of the file name below the var dir of the member.
func (m *member) readFile(t *testing.T, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(m.dir, filepath.FromSlash(name)))
	require.NoError(t, err)

	return string(content)
}

// request sends a request to the API of the member and returns the status code and the body.
func (m *member) request(t *testing.T, method string, path string, body io.Reader, contentType string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, "http://unix.socket"+path, body)
	require.NoError(t, err)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := m.client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, content
}

// jsonRequest sends a JSON request with the headers to the API of the member and returns the status code and the response headers.
func (m *member) jsonRequest(t *testing.T, method string, path string, content any, headers map[string]string) (int, http.Header) {
	t.Helper()

	var body io.Reader = http.NoBody
	if content != nil {
		data, err := json.Marshal(content)
		require.NoError(t, err)

		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, "http://unix.socket"+path, body)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := m.client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, resp.Header
}

// api sends a JSON request to the API of the member and returns the response.
func (m *member) api(t *testing.T, method string, path string, content any) api.Response {
	t.Helper()

	resp, err := m.tryAPI(method, path, content)
	require.NoError(t, err)

	return resp
}

// mustAPI is like api, but requires the request to succeed.
func (m *member) mustAPI(t *testing.T, method string, path string, content any) api.Response {
	t.Helper()

	resp := m.api(t, method, path, content)
	require.Empty(t, resp.Error, "%s %s on %s", method, path, m.name)

	return resp
}

// mustGet requests path like mustAPI and decodes the metadata of the response to target.
func (m *member) mustGet(t *testing.T, path string, target any) error {
	t.Helper()

	resp := m.mustAPI(t, http.MethodGet, path, nil)

	return resp.MetadataAsStruct(target)
}

// tryAPI is like api, but returns an error instead of failing the test, e.g. while the member restarts.
func (m *member) tryAPI(method string, path string, content any) (api.Response, error) {
	var body io.Reader
	if content != nil {
		data, err := json.Marshal(content)
		if err != nil {
			return api.Response{}, err
		}

		body = bytes.NewReader(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, "http://unix.socket"+path, body)
	if err != nil {
		return api.Response{}, err
	}

	req.Header.Set("Content-Type", "application/json")

	httpResp, err := m.client.Do(req)
	if err != nil {
		return api.Response{}, err
	}

	defer httpResp.Body.Close()

	var resp api.Response
	err = json.NewDecoder(httpResp.Body).Decode(&resp)
	if err != nil {
		return api.Response{}, fmt.Errorf("Failed to decode response of %s %s: %w", method, path, err)
	}

	return resp, nil
}

// members returns the cluster members as seen by m.
func (m *member) members(t *testing.T) []api.ClusterMember {
	t.Helper()

	members, err := m.tryMembers()
	require.NoError(t, err)

	return members
}

// tryMembers is like members, but returns an error instead of failing the test.
func (m *member) tryMembers() ([]api.ClusterMember, error) {
	resp, err := m.tryAPI(http.MethodGet, "/1.0/membership/members", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}

	var members []api.ClusterMember
	err = resp.MetadataAsStruct(&members)

	return members, err
}

// query runs a SQL statement on the global database.
func (m *member) query(t *testing.T, query string) {
	t.Helper()

	m.mustAPI(t, http.MethodPost, "/internal/sql", map[string]string{"query": query})
}

// hasFile reports whether the member stores the file name below its var dir.
func (m *member) hasFile(name string) bool {
	_, err := os.Stat(filepath.Join(m.dir, filepath.FromSlash(name)))
	return err == nil
}

// writeFile stores the file name below the var dir of the member.
func (m *member) writeFile(t *testing.T, name string, content string) {
	t.Helper()

	filename := filepath.Join(m.dir, filepath.FromSlash(name))

	err := os.MkdirAll(filepath.Dir(filename), 0o700)
	require.NoError(t, err)

	err = os.WriteFile(filename, []byte(content), 0o600)
	require.NoError(t, err)
}

// ageFile makes the file name below the var dir old enough to be pruned on startup.
func (m *member) ageFile(t *testing.T, name string) {
	t.Helper()

	old := time.Now().Add(-time.Hour)
	err := os.Chtimes(filepath.Join(m.dir, filepath.FromSlash(name)), old, old)
	require.NoError(t, err)
}

// logSize returns the current size of the log of the member.
func (m *member) logSize() int64 {
	fi, err := os.Stat(filepath.Join(m.dir, "daemon.log"))
	if err != nil {
		return 0
	}

	return fi.Size()
}

func (m *member) logTail() string {
	return lastLines(readFrom(filepath.Join(m.dir, "daemon.log"), 0), 50)
}

func readFrom(filename string, offset int64) string {
	content, err := os.ReadFile(filename)
	if err != nil || int64(len(content)) < offset {
		return ""
	}

	return string(content[offset:])
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n")
}

// httpsRequest sends a request to the network API of the member with the optional client certificate cert and returns the status code and the body.
func (m *member) httpsRequest(t *testing.T, cert *tls.Certificate, method string, path string, content any, headers map[string]string) (int, string) {
	t.Helper()

	var body io.Reader = http.NoBody
	if content != nil {
		data, err := json.Marshal(content)
		require.NoError(t, err)

		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, "https://"+m.address+path, body)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	if cert != nil {
		tlsConfig.Certificates = []tls.Certificate{*cert}
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}, Timeout: 2 * time.Minute}

	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(data)
}

// newClientCertificate returns a new client certificate and its PEM encoding.
func newClientCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()

	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	return cert, string(certPEM)
}
