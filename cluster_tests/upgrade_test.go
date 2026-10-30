package clustertests

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// lastSQLiteRelease is the last release storing its data in SQLite only.
const lastSQLiteRelease = "0.10.1"

// legacyBinary returns the daemon of a release before cowsql, skipping the test without one.
func legacyBinary(t *testing.T) string {
	t.Helper()

	binary := os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_LEGACY_BINARY")
	if binary == "" {
		t.Skipf("Set OPERATIONS_CENTER_CLUSTER_TEST_LEGACY_BINARY to the daemon of a release up to %s to run the upgrade test", lastSQLiteRelease)
	}

	binary, err := filepath.Abs(binary)
	require.NoError(t, err)

	out, err := exec.Command(binary, "--version").Output()
	require.NoError(t, err)

	version := strings.TrimSpace(string(out))
	require.True(t, releaseAtMost(t, version, lastSQLiteRelease), "The legacy daemon %q reports version %s, a release up to %s is required", binary, version, lastSQLiteRelease)

	return binary
}

// releaseAtMost reports whether the release version is not newer than limit.
func releaseAtMost(t *testing.T, version string, limit string) bool {
	t.Helper()

	parse := func(s string) [3]int {
		var v [3]int
		_, err := fmt.Sscanf(s, "%d.%d.%d", &v[0], &v[1], &v[2])
		require.NoError(t, err, "Invalid release version %q", s)
		return v
	}

	v, l := parse(version), parse(limit)
	for i := range v {
		if v[i] != l[i] {
			return v[i] < l[i]
		}
	}

	return true
}

// metadata returns the decoded metadata of a GET on m, so it can be compared across versions.
func metadata(t *testing.T, m *member, path string) any {
	t.Helper()

	var value any
	err := m.mustGet(t, path, &value)
	require.NoError(t, err)

	return value
}

// field returns the named field of the metadata of a GET on m.
func field(t *testing.T, m *member, path string, name string) any {
	t.Helper()

	object, ok := metadata(t, m, path).(map[string]any)
	require.True(t, ok, "%s is not an object", path)

	return object[name]
}

// putField changes one field of the config returned by a GET on path.
func putField(t *testing.T, m *member, path string, name string, value any) {
	t.Helper()

	object, ok := metadata(t, m, path).(map[string]any)
	require.True(t, ok, "%s is not an object", path)

	object[name] = value
	m.mustAPI(t, http.MethodPut, path, object)
}

// checksum returns the SHA-256 of the file name below the var dir of m.
func checksum(t *testing.T, m *member, name string) string {
	t.Helper()

	sum := sha256.Sum256([]byte(m.readFile(t, name)))

	return hex.EncodeToString(sum[:])
}

func TestUpgradeFromSQLite(t *testing.T) {
	binary, ip, basePort := testEnvironment(t)
	legacy := legacyBinary(t)

	c := newTestCluster(t)
	m := newMember(t, legacy, c.root, "m1", net.JoinHostPort(ip, strconv.Itoa(basePort)))
	c.members = []*member{m}
	m.start(t)

	// A standalone server of the old release is configured and filled with data through its API.
	trusted, _ := newClientCertificate(t)
	fingerprint := sha256.Sum256(trusted.Certificate[0])
	scriptlet := "def server_registration(candidate):\n    return True\n"

	m.mustAPI(t, http.MethodPut, "/1.0/system/network", map[string]string{"address": "https://" + m.address, "rest_server_address": m.address})
	putField(t, m, "/1.0/system/settings", "server_registration_scriptlet", scriptlet)
	putField(t, m, "/1.0/system/security", "trusted_tls_client_cert_fingerprints", []string{hex.EncodeToString(fingerprint[:])})
	putField(t, m, "/1.0/system/updates", "filter_expression", `version == "none"`)
	m.mustAPI(t, http.MethodPut, "/1.0/provisioning/channels/stable", map[string]string{"description": "before the upgrade"})
	m.mustAPI(t, http.MethodPost, "/1.0/provisioning/tokens", map[string]any{"uses_remaining": 3, "expire_at": "2030-01-01T00:00:00Z", "description": "before the upgrade"})
	m.mustAPI(t, http.MethodPost, "/1.0/provisioning/servers", map[string]string{"name": "server-before", "connection_url": "https://192.0.2.10:8443", "channel": "stable"})
	m.mustAPI(t, http.MethodPost, "/1.0/images/incus/sources", map[string]string{"name": "source-before", "url": "https://192.0.2.11", "filter_expression": "false"})
	filePath, apiPath := uploadImage(t, m, "before", "20260101")

	lists := []string{
		"/1.0/images/incus?recursion=1",
		"/1.0/images/incus/sources?recursion=1",
		"/1.0/provisioning/channels?recursion=1",
		"/1.0/provisioning/servers?recursion=1",
		"/1.0/provisioning/tokens?recursion=1",
	}

	before := map[string]any{}
	for _, path := range lists {
		before[path] = metadata(t, m, path)
	}

	_, imageContent := m.request(t, http.MethodGet, apiPath, nil, "")
	databaseSum := checksum(t, m, "local.db")

	status, body := m.httpsRequest(t, &trusted, http.MethodGet, "/1.0/images/incus", nil, nil)
	require.Equal(t, http.StatusOK, status, body)

	require.False(t, m.hasFile("database"), "The legacy daemon is expected to store its data in local.db only")
	m.stop()

	// The new release starts on the same directory.
	m.binary = binary
	m.start(t)

	for _, path := range lists {
		require.Equal(t, before[path], metadata(t, m, path), path)
	}

	require.Equal(t, "https://"+m.address, field(t, m, "/1.0/system/network", "address"))
	require.Equal(t, m.address, field(t, m, "/1.0/system/network", "rest_server_address"))
	require.Equal(t, scriptlet, field(t, m, "/1.0/system/settings", "server_registration_scriptlet"))
	require.Equal(t, []any{hex.EncodeToString(fingerprint[:])}, field(t, m, "/1.0/system/security", "trusted_tls_client_cert_fingerprints"))
	require.Equal(t, `version == "none"`, field(t, m, "/1.0/system/updates", "filter_expression"))

	status, content := m.request(t, http.MethodGet, apiPath, nil, "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, imageContent, content)
	require.True(t, m.hasFile(filePath))

	// The trusted client of the shared config keeps its access.
	status, body = m.httpsRequest(t, &trusted, http.MethodGet, "/1.0/images/incus", nil, nil)
	require.Equal(t, http.StatusOK, status, body)

	// The old database is kept unchanged next to the new one, the config stays in place.
	require.Equal(t, databaseSum, checksum(t, m, "local.db.migrated"))
	require.True(t, m.hasFile("config.yml"))
	require.False(t, m.hasFile("local.db"))
	require.True(t, m.hasFile("database/global"))
}
