package clustertests

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// testCluster is an Operations Center cluster of members run on the local host.
type testCluster struct {
	root    string
	members []*member
}

// testEnvironment returns the daemon, the address and the first port of the members, skipping the test unless the cluster tests are enabled.
func testEnvironment(t *testing.T) (binary string, ip string, basePort int) {
	t.Helper()

	if os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST") == "" {
		t.Skip("Set OPERATIONS_CENTER_CLUSTER_TEST to run the cluster tests")
	}

	binary = os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_BINARY")
	if binary == "" {
		binary = "../bin/operations-centerd"
	}

	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	require.FileExists(t, binary, `Build the daemon with "make build" or set OPERATIONS_CENTER_CLUSTER_TEST_BINARY`)

	ip = os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_ADDRESS")
	if ip == "" {
		ip = globalUnicastIP(t)
	}

	basePort = 17443
	if os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_PORT") != "" {
		basePort, err = strconv.Atoi(os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_PORT"))
		require.NoError(t, err)
	}

	return binary, ip, basePort
}

// newTestCluster returns an empty cluster below a temporary directory removed with the test.
func newTestCluster(t *testing.T) *testCluster {
	t.Helper()

	root, err := os.MkdirTemp("", "oc-cluster-")
	require.NoError(t, err)

	c := &testCluster{root: root}

	t.Cleanup(func() {
		for _, m := range c.members {
			if t.Failed() {
				t.Logf("Log of %s:\n%s", m.name, m.logTail())
			}

			m.kill()
		}

		if os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_NO_CLEANUP") == "" {
			_ = os.RemoveAll(root)
		} else {
			t.Logf("The members are kept in %q", root)
		}
	})

	return c
}

// globalUnicastIP returns the first global unicast IPv4 address of an interface that is up.
func globalUnicastIP(t *testing.T) string {
	t.Helper()

	interfaces, err := net.Interfaces()
	require.NoError(t, err)

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagRunning == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		require.NoError(t, err)

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok && ipNet.IP.To4() != nil && ipNet.IP.IsGlobalUnicast() {
				return ipNet.IP.String()
			}
		}
	}

	t.Fatal("No global unicast IPv4 address found, set OPERATIONS_CENTER_CLUSTER_TEST_ADDRESS")

	return ""
}

// uploadImage adds an image through m and returns the path of its file below the var dir and in the API.
func uploadImage(t *testing.T, m *member, osName string, version string) (filePath string, apiPath string) {
	t.Helper()

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="request_json"`},
		"Content-Type":        {"application/json"},
	})
	require.NoError(t, err)

	_, err = fmt.Fprintf(part, `{"os":%q,"release":"1","arch":"amd64","variant":"default","version":%q}`, osName, version)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	status, content := m.request(t, http.MethodPost, "/1.0/images/incus", body, w.FormDataContentType())
	require.Equal(t, http.StatusCreated, status, string(content))

	return fmt.Sprintf("images/%s/1/amd64/default/%s/incus.tar.xz", osName, version),
		fmt.Sprintf("/1.0/images/incus/%s:1:amd64:default/%s/incus.tar.xz", osName, version)
}
