package clustertests

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
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
	require.FileExists(t, binary, `Build the daemon with "make bld" or set OPERATIONS_CENTER_CLUSTER_TEST_BINARY`)

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

func setupCluster(t *testing.T) *testCluster {
	t.Helper()

	return setupClusterOf(t, 3)
}

// setupClusterOf returns a cluster of n members, three of them voters.
func setupClusterOf(t *testing.T, n int) *testCluster {
	t.Helper()

	binary, ip, basePort := testEnvironment(t)

	c := newTestCluster(t)
	for i := range n {
		address := net.JoinHostPort(ip, strconv.Itoa(basePort+i))
		c.members = append(c.members, newMember(t, binary, c.root, fmt.Sprintf("m%d", i+1), address))
	}

	first := c.members[0]
	first.start(t)
	first.mustAPI(t, http.MethodPost, "/1.0/membership/:enable", map[string]string{
		"server_name":     first.name,
		"cluster_address": first.address,
	})

	// The member restarts to enable clustering.
	c.waitFor(t, "first member to be clustered", func() bool {
		members, err := first.tryMembers()
		return err == nil && len(members) == 1
	})

	for _, m := range c.members[1:] {
		m.start(t)
		c.join(t, first, m)
	}

	// A just promoted member does not take part in elections until it knows about it.
	c.waitFor(t, "three members to be voters", func() bool {
		for _, m := range c.members {
			members, err := m.tryMembers()
			if err != nil || len(members) != len(c.members) || voters(members) != min(len(c.members), 3) {
				return false
			}
		}

		return true
	})

	return c
}

// join adds the running standalone member m to the cluster through via and waits until all members trust it.
func (c *testCluster) join(t *testing.T, via *member, m *member) {
	t.Helper()

	c.joinWith(t, via, m, false)
}

// joinWith is join with the force flag, which discards the data of m.
func (c *testCluster) joinWith(t *testing.T, via *member, m *member, force bool) {
	t.Helper()

	offsets := map[*member]int64{}
	for _, other := range c.members {
		offsets[other] = other.logSize()
	}

	via.mustAPI(t, http.MethodPost, "/1.0/membership/tokens", map[string]string{
		"server_name": m.name,
		"expiry":      "1h",
	})

	var token string
	resp := via.mustAPI(t, http.MethodGet, "/1.0/membership/tokens/"+m.name, nil)
	err := resp.MetadataAsStruct(&token)
	require.NoError(t, err)

	m.mustAPI(t, http.MethodPost, "/1.0/membership/:join", map[string]any{
		"server_name":     m.name,
		"cluster_address": m.address,
		"token":           token,
		"force":           force,
	})

	if !slices.Contains(c.members, m) {
		c.members = append(c.members, m)
	}

	c.waitForOnline(t, m)

	// The members trust a new member once they got a heartbeat listing it.
	c.waitFor(t, "all members to trust "+m.name, func() bool {
		for _, other := range c.members {
			if other == m || other.cmd == nil {
				continue
			}

			if !strings.Contains(readFrom(filepath.Join(other.dir, "daemon.log"), offsets[other]), "Cluster member states changed, updating authentication") {
				return false
			}
		}

		return true
	})
}

// voters returns the number of voters among members.
func voters(members []api.ClusterMember) int {
	n := 0
	for _, member := range members {
		if slices.Equal(member.Roles, []string{"voter"}) {
			n++
		}
	}

	return n
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

func (c *testCluster) waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(time.Second)
	}

	t.Fatalf("Timeout waiting for %s", what)
}

// waitForOnline waits until all running members see m online.
func (c *testCluster) waitForOnline(t *testing.T, m *member) {
	t.Helper()

	c.waitFor(t, m.name+" to be online", func() bool {
		for _, other := range c.members {
			if other.cmd == nil {
				continue
			}

			members, err := other.tryMembers()
			if err != nil {
				return false
			}

			online := false
			for _, member := range members {
				if member.Name == m.name && !member.Offline {
					online = true
				}
			}

			if !online {
				return false
			}
		}

		return true
	})
}

// waitForAllOnline waits for every running member to be reported online by all members.
func (c *testCluster) waitForAllOnline(t *testing.T) {
	t.Helper()

	for _, m := range c.members {
		if m.cmd != nil {
			c.waitForOnline(t, m)
		}
	}
}

// waitForDatabase waits until m can access the database, e.g. while a new leader is elected.
func (c *testCluster) waitForDatabase(t *testing.T, m *member) {
	t.Helper()

	c.waitFor(t, m.name+" to access the database", func() bool {
		_, err := m.tryMembers()
		return err == nil
	})
}

// byName returns the member called name.
func (c *testCluster) byName(t *testing.T, name string) *member {
	t.Helper()

	for _, m := range c.members {
		if m.name == name {
			return m
		}
	}

	t.Fatalf("No member %q", name)

	return nil
}

// others returns the members except m.
func (c *testCluster) others(m *member) []*member {
	var others []*member
	for _, other := range c.members {
		if other != m {
			others = append(others, other)
		}
	}

	return others
}

// fileTarget returns the only member storing the file name below its var dir.
func (c *testCluster) fileTarget(t *testing.T, name string) *member {
	t.Helper()

	var targets []*member
	for _, m := range c.members {
		if m.hasFile(name) {
			targets = append(targets, m)
		}
	}

	require.Len(t, targets, 1, "File %q is expected on a single member", name)

	return targets[0]
}

// leader returns the cluster leader.
func (c *testCluster) leader(t *testing.T) *member {
	t.Helper()

	for _, clusterMember := range c.members[0].members(t) {
		if !clusterMember.Leader {
			continue
		}

		for _, m := range c.members {
			if m.name == clusterMember.Name {
				return m
			}
		}
	}

	t.Fatal("No cluster leader found")

	return nil
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

func TestCluster(t *testing.T) {
	c := setupCluster(t)

	t.Run("all members are voters", func(t *testing.T) {
		members := c.members[0].members(t)
		require.Len(t, members, 3)

		for _, m := range members {
			require.Equal(t, []string{"voter"}, m.Roles, m.Name)
			require.False(t, m.Offline, m.Name)
		}
	})

	t.Run("all members share the client keypair", func(t *testing.T) {
		for _, m := range c.members[1:] {
			require.Equal(t, c.members[0].clientCertificate(t), m.clientCertificate(t), m.name)
		}
	})

	t.Run("the address of Operations Center defaults to the cluster address of the first member", func(t *testing.T) {
		for _, m := range c.members {
			resp := m.mustAPI(t, http.MethodGet, "/1.0/system/network", nil)

			var network apisystem.Network
			require.NoError(t, resp.MetadataAsStruct(&network))
			require.Equal(t, "https://"+c.members[0].address, network.OperationsCenterAddress, m.name)
		}
	})

	t.Run("a member stays reachable, while its https listener covers its cluster address", func(t *testing.T) {
		m := c.members[1]

		setRestServerAddress := func(address string) {
			resp := m.mustAPI(t, http.MethodGet, "/1.0/system/network", nil)

			var network apisystem.Network
			require.NoError(t, resp.MetadataAsStruct(&network))

			network.RestServerAddress = address
			m.mustAPI(t, http.MethodPut, "/1.0/system/network", apisystem.NetworkPut{
				NetworkLocalPut:  network.NetworkLocalPut,
				NetworkGlobalPut: network.NetworkGlobalPut,
			})
		}

		// The https listener takes over the address of the cluster listener without a restart and gives it back.
		for _, address := range []string{m.address, ""} {
			changed := time.Now()
			setRestServerAddress(address)

			// The heartbeats of the leader still reach the member on its address.
			c.waitFor(t, "a heartbeat of "+m.name, func() bool {
				for _, member := range c.members[0].members(t) {
					if member.Name == m.name {
						return member.LastHeartbeat.After(changed)
					}
				}

				return false
			})
		}
	})

	t.Run("a clustered member refuses enabling and joining itself", func(t *testing.T) {
		clusterKeypair := func(m *member) string {
			certificate, err := os.ReadFile(filepath.Join(m.dir, "cluster.crt"))
			require.NoError(t, err)

			key, err := os.ReadFile(filepath.Join(m.dir, "cluster.key"))
			require.NoError(t, err)

			return string(certificate) + string(key)
		}

		keypairs := map[*member]string{}
		offsets := map[*member]int64{}
		for _, m := range c.members {
			keypairs[m] = clusterKeypair(m)
			offsets[m] = m.logSize()
		}

		for _, m := range c.members {
			// The server record of the member is named after it, which refuses the name before the bootstrap does.
			resp := m.api(t, http.MethodPost, "/1.0/membership/:enable", map[string]string{"server_name": m.name, "cluster_address": m.address})
			require.Contains(t, resp.Error, fmt.Sprintf("The name %q is used by a server already", m.name), m.name)

			resp = m.api(t, http.MethodPost, "/1.0/membership/:join", map[string]string{"server_name": m.name, "cluster_address": m.address, "token": "token"})
			require.Contains(t, resp.Error, "already clustered", m.name)
		}

		for _, m := range c.members {
			require.Equal(t, keypairs[m], clusterKeypair(m), m.name)

			// Requests through the unix socket are handled by the receiving member without forwarding.
			for line := range strings.SplitSeq(readFrom(filepath.Join(m.dir, "daemon.log"), offsets[m]), "\n") {
				if strings.Contains(line, `msg="access log"`) && strings.Contains(line, "request_uri=/1.0/membership/:") {
					require.Contains(t, line, "ip=@", m.name)
				}
			}
		}
	})

	t.Run("the cluster stays available, while a member is down", func(t *testing.T) {
		for _, m := range c.members {
			m.kill()

			for _, other := range c.members {
				if other != m {
					c.waitForDatabase(t, other)
				}
			}

			m.start(t)
			c.waitForOnline(t, m)
		}
	})

	t.Run("a stopped leader hands over its role and closes the database", func(t *testing.T) {
		leader := c.leader(t)
		offset := leader.logSize()

		leader.stop()

		log := readFrom(filepath.Join(leader.dir, "daemon.log"), offset)
		require.Contains(t, log, "Daemon shutdown completed successfully")
		for _, msg := range []string{"Task(s) still running", "Failed to hand over", "Timed out running closer"} {
			require.NotContains(t, log, msg)
		}

		other := c.others(leader)[0]
		c.waitForDatabase(t, other)
		for _, member := range other.members(t) {
			if member.Leader {
				require.NotEqual(t, leader.name, member.Name)
			}
		}

		leader.start(t)
		c.waitForOnline(t, leader)
	})

	t.Run("files are read through any member", func(t *testing.T) {
		filePath, apiPath := uploadImage(t, c.members[1], "one", "20260101")
		fileTarget := c.fileTarget(t, filePath)

		contents := make([]string, 0, len(c.members))
		for _, m := range c.members {
			status, content := m.request(t, http.MethodGet, apiPath, nil, "")
			require.Equal(t, http.StatusOK, status, m.name)

			contents = append(contents, string(content))
		}

		require.Equal(t, contents[0], contents[1])
		require.Equal(t, contents[0], contents[2])

		// The file is not available while its file target is down.
		fileTarget.kill()

		for _, m := range c.others(fileTarget) {
			c.waitFor(t, m.name+" to report the file as unavailable", func() bool {
				resp, err := m.tryAPI(http.MethodGet, apiPath, nil)
				return err == nil && resp.Code == http.StatusServiceUnavailable && strings.Contains(resp.Error, strconv.Quote(fileTarget.name))
			})
		}

		fileTarget.start(t)
		c.waitForOnline(t, fileTarget)
	})

	t.Run("requests for the files of a member, which is down, are refused", func(t *testing.T) {
		filePath, apiPath := uploadImage(t, c.members[2], "two", "20260202")
		fileTarget := c.fileTarget(t, filePath)
		other := c.others(fileTarget)[0]

		fileTarget.stop()
		c.waitForDatabase(t, other)

		resp := other.api(t, http.MethodDelete, "/1.0/images/incus/two:1:amd64:default", nil)
		require.Equal(t, http.StatusServiceUnavailable, resp.Code, resp.Error)
		require.Contains(t, resp.Error, strconv.Quote(fileTarget.name))

		fileTarget.start(t)
		c.waitForOnline(t, fileTarget)

		other.mustAPI(t, http.MethodDelete, "/1.0/images/incus/two:1:amd64:default", nil)
		require.False(t, fileTarget.hasFile(filePath))

		status, _ := other.request(t, http.MethodGet, apiPath, nil, "")
		require.Equal(t, http.StatusNotFound, status)
	})

	t.Run("update files are read through any member", func(t *testing.T) {
		const updateUUID = "2d6c7b0e-3f1a-4e5b-9c8d-7a6b5c4d3e2f"

		fileTarget := c.members[1]
		fileTarget.writeFile(t, "updates/"+updateUUID+"/x86_64/update.raw", "update\n")
		c.members[0].query(t, `INSERT INTO updates (uuid, origin, version, published_at, severity, upstream_channels, files, status, member_id) VALUES ('`+updateUUID+`', 'test', '1', CURRENT_TIMESTAMP, 'none', '[]', '[{"filename":"x86_64/update.raw","size":7}]', 'ready', (SELECT id FROM cluster_members WHERE name = '`+fileTarget.name+`'))`)

		for _, m := range c.members {
			status, content := m.request(t, http.MethodGet, "/1.0/provisioning/updates/"+updateUUID+"/files/x86_64/update.raw", nil, "")
			require.Equal(t, http.StatusOK, status, m.name)
			require.Equal(t, "update\n", string(content), m.name)
		}
	})

	t.Run("cluster artifacts survive renames and are deleted on all members", func(t *testing.T) {
		m := c.members[0]

		const clusterUUID = "7b1a7c3e-6c1d-4a51-9d39-1f0c1d7c2a01"
		m.query(t, `INSERT INTO clusters (name, connection_url, status, last_updated, channel_id, uuid) VALUES ('one', 'https://192.0.2.1:8443', 'pending', CURRENT_TIMESTAMP, 1, '`+clusterUUID+`')`)

		// The artifact files are stored by a single member.
		const filePath = "artifacts/" + clusterUUID + "/terraform-configuration/main.tf"
		fileTarget := c.members[2]
		fileTarget.writeFile(t, filePath, "tf\n")
		m.query(t, `INSERT INTO cluster_artifacts (cluster_id, name, description, properties, files, last_updated, member_id) VALUES ((SELECT id FROM clusters WHERE name = 'one'), 'terraform-configuration', '', '{}', '[{"Name":"main.tf","MimeType":"text/plain","Size":3}]', CURRENT_TIMESTAMP, (SELECT id FROM cluster_members WHERE name = '`+fileTarget.name+`'))`)

		for _, m := range c.members {
			status, content := m.request(t, http.MethodGet, "/1.0/provisioning/clusters/one/artifacts/terraform-configuration/main.tf", nil, "")
			require.Equal(t, http.StatusOK, status, m.name)
			require.Equal(t, "tf\n", string(content), m.name)
		}

		// The leader serves the clusters, since it keeps the progress of their updates.
		leader := c.leader(t)
		offset := leader.logSize()
		for _, other := range c.others(leader) {
			var cluster api.Cluster
			require.NoError(t, other.mustGet(t, "/1.0/provisioning/clusters/one", &cluster))
			require.Equal(t, "one", cluster.Name, other.name)

			var clusters []api.Cluster
			require.NoError(t, other.mustGet(t, "/1.0/provisioning/clusters?recursion=1", &clusters))
			require.Len(t, clusters, 1, other.name)
		}

		accessLog := func(uri string) int {
			count := 0
			for line := range strings.SplitSeq(readFrom(filepath.Join(leader.dir, "daemon.log"), offset), "\n") {
				if strings.Contains(line, `msg="access log"`) && strings.Contains(line, "method=GET request_uri="+uri+" ") {
					count++
				}
			}

			return count
		}

		require.Equal(t, len(c.others(leader)), accessLog("/1.0/provisioning/clusters/one"))
		require.Equal(t, len(c.others(leader)), accessLog(`"/1.0/provisioning/clusters?recursion=1"`))

		// A rename does not touch the files, so it works while a member is down.
		fileTarget.stop()
		c.waitForDatabase(t, m)
		m.mustAPI(t, http.MethodPost, "/1.0/provisioning/clusters/one", map[string]string{"name": "two"})
		fileTarget.start(t)
		c.waitForOnline(t, fileTarget)

		for _, m := range c.members {
			status, _ := m.request(t, http.MethodGet, "/1.0/provisioning/clusters/two/artifacts/terraform-configuration/main.tf", nil, "")
			require.Equal(t, http.StatusOK, status, m.name)
		}

		// The delete removes the files on all members, a stopped member removes them on startup.
		fileTarget.stop()
		c.waitForDatabase(t, c.members[1])
		c.members[1].mustAPI(t, http.MethodDelete, "/1.0/provisioning/clusters/two", nil)
		require.True(t, fileTarget.hasFile(filePath))

		fileTarget.ageFile(t, filePath)
		fileTarget.start(t)
		c.waitForOnline(t, fileTarget)
		require.False(t, fileTarget.hasFile(filePath))
	})

	t.Run("every member has its own server record", func(t *testing.T) {
		records := func() map[string]api.Server {
			var servers []api.Server

			resp := c.members[0].mustAPI(t, http.MethodGet, "/1.0/provisioning/servers?recursion=1", nil)
			require.NoError(t, resp.MetadataAsStruct(&servers))

			records := map[string]api.Server{}
			for _, server := range servers {
				if server.Type == api.ServerTypeOperationsCenter {
					records[server.Name] = server
				}
			}

			return records
		}

		c.waitFor(t, "the server records of all members", func() bool {
			return len(records()) == len(c.members)
		})

		// The record of the standalone server has been renamed to the first member.
		for _, m := range c.members {
			record, ok := records()[m.name]
			require.True(t, ok, m.name)
			require.Equal(t, "https://"+m.address, record.ConnectionURL, m.name)
			require.Equal(t, m.readFile(t, "server.crt"), record.Certificate, m.name)
		}

		// The member maintains the fields of its record that follow the cluster, the others can be changed.
		member := c.members[1]
		record := records()[member.name]
		path := "/1.0/provisioning/servers/" + member.name
		put := api.ServerPut{PublicConnectionURL: record.PublicConnectionURL, Channel: record.Channel, Description: "changed"}
		c.members[0].mustAPI(t, http.MethodPut, path, put)
		require.Equal(t, "changed", records()[member.name].Description)

		for _, req := range []struct {
			method  string
			content any
			error   string
		}{
			{method: http.MethodPut, content: api.ServerPut{PublicConnectionURL: "https://other:8443", Channel: record.Channel}, error: "follows the shared address"},
			{method: http.MethodPut, content: api.ServerPut{PublicConnectionURL: record.PublicConnectionURL, Channel: "other"}, error: "follows the default update channel"},
			{method: http.MethodPost, content: map[string]string{"name": "renamed"}, error: "can not be renamed"},
			{method: http.MethodDelete, error: "can not be deleted"},
		} {
			resp := c.members[0].api(t, req.method, path, req.content)
			require.Equal(t, http.StatusBadRequest, resp.Code, req.method)
			require.Contains(t, resp.Error, req.error, req.method)
		}

		// The name of a server can not be used for a member, since the record of the member is named after it.
		c.members[0].mustAPI(t, http.MethodPost, "/1.0/provisioning/servers", map[string]string{"name": "taken", "channel": "stable"})
		resp := c.members[0].api(t, http.MethodPost, "/1.0/membership/tokens", map[string]string{"server_name": "taken", "expiry": "1h"})
		require.Equal(t, http.StatusBadRequest, resp.Code)
		require.Contains(t, resp.Error, `The name "taken" is used by a server already`)

		c.members[0].mustAPI(t, http.MethodDelete, "/1.0/provisioning/servers/taken", nil)
	})

	t.Run("forwarded requests keep the identity of their client", func(t *testing.T) {
		leader := c.leader(t)

		leader.mustAPI(t, http.MethodPost, "/1.0/provisioning/tokens", map[string]any{"uses_remaining": 10, "expire_at": time.Now().Add(time.Hour)})

		var tokens []api.Token
		require.NoError(t, leader.mustGet(t, "/1.0/provisioning/tokens?recursion=1", &tokens))
		require.NotEmpty(t, tokens)

		token := tokens[0].UUID.String()
		leader.mustAPI(t, http.MethodPost, "/1.0/provisioning/tokens/"+token+"/seeds", map[string]any{"name": "private", "public": false, "seeds": map[string]any{}})

		anonymous, _ := newClientCertificate(t)

		// A client can not pass on an identity itself.
		forged := map[string]string{
			"X-OperationsCenter-forwarded-username": "root",
			"X-OperationsCenter-forwarded-protocol": "unix",
			"X-OperationsCenter-forwarded-address":  "@",
		}

		for _, m := range c.members {
			for _, headers := range []map[string]string{nil, forged} {
				status, body := m.httpsRequest(t, &anonymous, http.MethodPost, "/1.0/provisioning/servers", map[string]string{"name": "anonymous", "channel": "stable"}, headers)
				require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status, "%s: %s", m.name, body)

				status, body = m.httpsRequest(t, nil, http.MethodGet, "/1.0/provisioning/tokens/"+token+"/seeds/private/architecture/x86_64/type/iso/file.iso", nil, headers)
				require.Equal(t, http.StatusForbidden, status, "%s: %s", m.name, body)
			}
		}

		status, _ := leader.request(t, http.MethodGet, "/1.0/provisioning/servers/anonymous", nil, "")
		require.Equal(t, http.StatusNotFound, status)

		// Servers registering through a non-leader member are identified by their own certificate.
		via := c.others(leader)[0]

		certificates := map[string]string{}
		clientCertificates := map[string]tls.Certificate{}
		for _, name := range []string{"server-a", "server-b"} {
			cert, certPEM := newClientCertificate(t)
			certificates[name] = certPEM
			clientCertificates[name] = cert

			status, body := via.httpsRequest(t, &cert, http.MethodPost, "/1.0/provisioning/servers?token="+token, map[string]string{"name": name, "connection_url": "https://192.0.2.10:8443"}, nil)
			require.Equal(t, http.StatusCreated, status, body)

			var server api.Server
			require.NoError(t, leader.mustGet(t, "/1.0/provisioning/servers/"+name, &server))
			require.Equal(t, certPEM, server.Certificate, name)
		}

		cert := clientCertificates["server-b"]
		status, body := via.httpsRequest(t, &cert, http.MethodPut, "/1.0/provisioning/servers/:self", map[string]string{"connection_url": "https://192.0.2.22:8443"}, nil)
		require.Equal(t, http.StatusOK, status, body)

		for name, want := range map[string]string{"server-a": "https://192.0.2.10:8443", "server-b": "https://192.0.2.22:8443"} {
			var server api.Server
			require.NoError(t, leader.mustGet(t, "/1.0/provisioning/servers/"+name, &server))
			require.Equal(t, want, server.ConnectionURL, name)

			leader.mustAPI(t, http.MethodDelete, "/1.0/provisioning/servers/"+name, nil)
		}
	})

	t.Run("updates checking the ETag are handled by the leader through any member", func(t *testing.T) {
		leader := c.leader(t)
		via := c.others(leader)[0]

		status, header := via.jsonRequest(t, http.MethodGet, "/1.0/provisioning/channels/stable", nil, nil)
		require.Equal(t, http.StatusOK, status)

		etag := header.Get("ETag")
		require.NotEmpty(t, etag)

		status, _ = via.jsonRequest(t, http.MethodPut, "/1.0/provisioning/channels/stable", api.ChannelPut{Description: "changed"}, map[string]string{"If-Match": etag})
		require.Equal(t, http.StatusOK, status)

		// The ETag does not match the changed channel anymore.
		status, _ = via.jsonRequest(t, http.MethodPut, "/1.0/provisioning/channels/stable", api.ChannelPut{Description: "changed again"}, map[string]string{"If-Match": etag})
		require.Equal(t, http.StatusPreconditionFailed, status)

		var channel api.Channel
		require.NoError(t, leader.mustGet(t, "/1.0/provisioning/channels/stable", &channel))
		require.Equal(t, "changed", channel.Description)
	})

	t.Run("only cluster members manage the membership and the files of a member", func(t *testing.T) {
		leader := c.leader(t)

		var security map[string]any
		require.NoError(t, leader.mustGet(t, "/1.0/system/security", &security))

		previous := security["trusted_tls_client_cert_fingerprints"]

		trusted, _ := newClientCertificate(t)
		fingerprint := sha256.Sum256(trusted.Certificate[0])
		security["trusted_tls_client_cert_fingerprints"] = []string{hex.EncodeToString(fingerprint[:])}
		leader.mustAPI(t, http.MethodPut, "/1.0/system/security", security)

		for _, m := range c.members {
			status, body := m.httpsRequest(t, &trusted, http.MethodGet, "/1.0/system/security", nil, nil)
			require.Equal(t, http.StatusOK, status, "%s: %s", m.name, body)

			for _, path := range []string{"/internal/membership/:reset", "/internal/membership/:rebalance", "/internal/updates/:check", "/internal/images/name/:refresh"} {
				status, body = m.httpsRequest(t, &trusted, http.MethodPost, path, nil, nil)
				require.Equal(t, http.StatusForbidden, status, "%s %s: %s", m.name, path, body)
			}

			for _, method := range []string{http.MethodGet, http.MethodDelete} {
				status, body = m.httpsRequest(t, &trusted, method, "/internal/files/images", nil, nil)
				require.Equal(t, http.StatusForbidden, status, "%s %s: %s", m.name, method, body)
			}

			// The local unix socket keeps access to the files of its member.
			status, _ = m.request(t, http.MethodGet, "/internal/files/updates", nil, "")
			require.NotEqual(t, http.StatusForbidden, status, "%s through the unix socket", m.name)
		}

		// A member pulls files from the other members only, not from any address it is told.
		memberCert := c.others(leader)[0].serverCertificate(t)
		status, body := leader.httpsRequest(t, &memberCert, http.MethodPost, "/internal/files/images/pulled?source=192.0.2.99:8443", nil, nil)
		require.Equal(t, http.StatusBadRequest, status, body)
		require.Contains(t, body, "not the address of a cluster member")

		security["trusted_tls_client_cert_fingerprints"] = previous
		leader.mustAPI(t, http.MethodPut, "/1.0/system/security", security)
	})

	t.Run("a client can not keep the leader from forwarding to the member storing the files", func(t *testing.T) {
		leader := c.leader(t)
		others := c.others(leader)
		holder, client := others[0], others[1]

		filePath, _ := uploadImage(t, client, "marked", "20260505")
		holder.mustAPI(t, http.MethodPost, "/1.0/membership/members/"+leader.name+"/:move-files", map[string]string{"target": holder.name})
		require.True(t, holder.hasFile(filePath))

		// The marker of a request forwarded to a member, set by the client.
		status, _ := client.jsonRequest(t, http.MethodDelete, "/1.0/images/incus/marked:1:amd64:default", nil, map[string]string{"X-OperationsCenter-forwarded-to-member": "1"})
		require.Equal(t, http.StatusOK, status)
		require.False(t, holder.hasFile(filePath), "the member storing the files deletes them")
	})

	t.Run("a request forwarded to a member, which is no longer the leader, is not forwarded again", func(t *testing.T) {
		leader := c.leader(t)
		others := c.others(leader)
		sender, receiver := others[0], others[1]

		memberCert, err := tls.LoadX509KeyPair(filepath.Join(sender.dir, "server.crt"), filepath.Join(sender.dir, "server.key"))
		require.NoError(t, err)

		var security map[string]any
		require.NoError(t, leader.mustGet(t, "/1.0/system/security", &security))

		status, body := receiver.httpsRequest(t, &memberCert, http.MethodPut, "/1.0/system/security", security, map[string]string{"X-OperationsCenter-forwarded-to-leader": "1"})
		require.Equal(t, http.StatusServiceUnavailable, status, body)
		require.Contains(t, body, "The cluster leader changed while the request was forwarded")

		status, body = receiver.httpsRequest(t, &memberCert, http.MethodPut, "/1.0/system/security", security, nil)
		require.Equal(t, http.StatusOK, status, "the same request from a member without the marker is forwarded to the leader: %s", body)
	})

	t.Run("changes during an election are refused as unavailable", func(t *testing.T) {
		leader := c.leader(t)
		other := c.others(leader)[0]

		channel := map[string]string{"description": "changed during an election"}

		// The internal membership routes forward to the leader on their own.
		memberCert := other.serverCertificate(t)
		rebalance := func() api.Response {
			status, body := other.httpsRequest(t, &memberCert, http.MethodPost, "/internal/membership/:rebalance", nil, nil)

			var resp api.Response
			require.NoError(t, json.Unmarshal([]byte(body), &resp), "%d: %s", status, body)

			return resp
		}

		require.Empty(t, rebalance().Error)

		leader.kill()

		var answered []int
		var messages []string
		var membershipMessages []string
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			membershipResp := rebalance()
			if membershipResp.Error != "" {
				require.Equal(t, http.StatusServiceUnavailable, membershipResp.Code, membershipResp.Error)
				require.Contains(t, membershipResp.Error, "cluster leader", "the answer names the leader")

				if !slices.Contains(membershipMessages, membershipResp.Error) {
					membershipMessages = append(membershipMessages, membershipResp.Error)
				}
			}

			resp, err := other.tryAPI(http.MethodPut, "/1.0/provisioning/channels/stable", channel)
			require.NoError(t, err)

			if resp.Code == 0 {
				break
			}

			if !slices.Contains(messages, resp.Error) {
				messages = append(messages, resp.Error)
			}

			answered = append(answered, resp.Code)
			require.Equal(t, http.StatusServiceUnavailable, resp.Code, resp.Error)
			require.Contains(t, resp.Error, "cluster leader", "the answer names the leader")
			require.NotContains(t, resp.Error, "files")

			time.Sleep(250 * time.Millisecond)
		}

		require.NotEmpty(t, answered, "the change is refused while there is no leader")
		require.NotEmpty(t, membershipMessages, "the membership request is refused while there is no leader")
		t.Logf("Answers while there was no leader: %q, to membership requests: %q", messages, membershipMessages)

		resp, err := other.tryAPI(http.MethodPut, "/1.0/provisioning/channels/stable", channel)
		require.NoError(t, err)
		require.Zero(t, resp.Code, "the new leader takes the change: %s", resp.Error)

		leader.start(t)
		c.waitForAllOnline(t)
	})

	t.Run("a join token is only consumed with its secret", func(t *testing.T) {
		leader := c.leader(t)

		leader.mustAPI(t, http.MethodPost, "/1.0/membership/tokens", map[string]string{"server_name": "joiner", "expiry": "1h"})

		var token string
		require.NoError(t, leader.mustGet(t, "/1.0/membership/tokens/joiner", &token))

		forged, err := api.DecodeJoinToken(token)
		require.NoError(t, err)

		forged.Secret = strings.Repeat("0", len(forged.Secret))
		forgedToken, err := forged.String()
		require.NoError(t, err)

		untrusted, _ := newClientCertificate(t)
		status, body := leader.httpsRequest(t, &untrusted, http.MethodPost, "/internal/membership/:trust", map[string]string{"token": forgedToken}, nil)
		require.Equal(t, http.StatusForbidden, status, body)

		// A token for an unknown name is refused the same way, so the pending members can not be enumerated.
		forged.ServerName = "unknown"
		unknownToken, err := forged.String()
		require.NoError(t, err)

		unknownStatus, unknownBody := leader.httpsRequest(t, &untrusted, http.MethodPost, "/internal/membership/:trust", map[string]string{"token": unknownToken}, nil)
		require.Equal(t, http.StatusForbidden, unknownStatus, unknownBody)
		require.Equal(t, errorMessage(t, body), errorMessage(t, unknownBody))

		status, body = leader.httpsRequest(t, &untrusted, http.MethodPost, "/internal/membership/:trust", map[string]string{"token": "invalid"}, nil)
		require.Equal(t, http.StatusBadRequest, status, body)
		require.Contains(t, body, "Invalid join token")

		var kept string
		require.NoError(t, leader.mustGet(t, "/1.0/membership/tokens/joiner", &kept))
		require.Equal(t, token, kept)

		leader.mustAPI(t, http.MethodDelete, "/1.0/membership/tokens/joiner", nil)
	})

	t.Run("shared config changes are reverted, if a member does not apply them", func(t *testing.T) {
		leader := c.leader(t)
		paused := c.others(leader)[1]

		fingerprints := func(m *member) string {
			var security map[string]any
			require.NoError(t, m.mustGet(t, "/1.0/system/security", &security))

			return fmt.Sprint(security["trusted_tls_client_cert_fingerprints"])
		}

		var security map[string]any
		require.NoError(t, leader.mustGet(t, "/1.0/system/security", &security))

		previous := fingerprints(leader)
		security["trusted_tls_client_cert_fingerprints"] = []string{strings.Repeat("cd", 32)}

		require.NoError(t, paused.cmd.Process.Signal(syscall.SIGSTOP))

		resp := leader.api(t, http.MethodPut, "/1.0/system/security", security)

		require.NoError(t, paused.cmd.Process.Signal(syscall.SIGCONT))
		require.Contains(t, resp.Error, strconv.Quote(paused.name))
		require.Contains(t, resp.Error, "reverted")

		for _, m := range c.members {
			require.Equal(t, previous, fingerprints(m), m.name)
		}

		// Once all members are online again, the change is applied by all of them.
		c.waitForAllOnline(t)
		leader.mustAPI(t, http.MethodPut, "/1.0/system/security", security)

		for _, m := range c.members {
			require.Equal(t, fmt.Sprint([]string{strings.Repeat("cd", 32)}), fingerprints(m), m.name)
		}
	})

	t.Run("secure boot media is served by any member", func(t *testing.T) {
		leader := c.leader(t)
		holder := c.others(leader)[0]

		holder.writeFile(t, "secure-boot-media/abcdefghijkl.iso", "secure boot enrollment media")
		leader.query(t, `INSERT INTO media (type, name, member_id, config, expires_at) VALUES ('secure-boot', 'abcdefghijkl', (SELECT id FROM cluster_members WHERE name = '`+holder.name+`'), '{}', datetime('now', '+1 hour'))`)

		for _, m := range c.members {
			status, content := m.request(t, http.MethodGet, "/1.0/provisioning/secure-boot-media/abcdefghijkl.iso", nil, "")
			require.Equal(t, http.StatusOK, status, m.name)
			require.Equal(t, "secure boot enrollment media", string(content), m.name)

			status, partial := m.httpsRequest(t, nil, http.MethodGet, "/1.0/provisioning/secure-boot-media/abcdefghijkl.iso", nil, map[string]string{"Range": "bytes=7-10"})
			require.Equal(t, http.StatusPartialContent, status, m.name)
			require.Equal(t, "boot", partial, m.name)

			status, _ = m.request(t, http.MethodGet, "/1.0/provisioning/secure-boot-media/zzzzzzzzzzzz.iso", nil, "")
			require.Equal(t, http.StatusNotFound, status, m.name)
		}

		// The media of a member, which is down, is unavailable rather than missing.
		holder.kill()
		status, _ := leader.request(t, http.MethodGet, "/1.0/provisioning/secure-boot-media/abcdefghijkl.iso", nil, "")
		require.Equal(t, http.StatusServiceUnavailable, status)

		holder.start(t)
		c.waitForOnline(t, holder)
	})

	t.Run("a member, which missed a shared config change, does not overwrite it", func(t *testing.T) {
		leader := c.leader(t)
		stale := c.others(leader)[1]

		fingerprints := func(m *member) string {
			var security map[string]any
			require.NoError(t, m.mustGet(t, "/1.0/system/security", &security))

			return fmt.Sprint(security["trusted_tls_client_cert_fingerprints"])
		}

		var security map[string]any
		require.NoError(t, leader.mustGet(t, "/1.0/system/security", &security))
		previous := security["trusted_tls_client_cert_fingerprints"]

		require.NoError(t, stale.cmd.Process.Signal(syscall.SIGSTOP))
		c.waitFor(t, stale.name+" to be offline", func() bool {
			members, err := leader.tryMembers()
			if err != nil {
				return false
			}

			for _, member := range members {
				if member.Name == stale.name {
					return member.Offline
				}
			}

			return false
		})

		// The change skips the offline member.
		security["trusted_tls_client_cert_fingerprints"] = []string{strings.Repeat("ef", 32)}
		leader.mustAPI(t, http.MethodPut, "/1.0/system/security", security)

		require.NoError(t, stale.cmd.Process.Signal(syscall.SIGCONT))
		c.waitForAllOnline(t)

		// The settings are changed by the member receiving them, which still holds the old security config.
		var settings map[string]any
		require.NoError(t, stale.mustGet(t, "/1.0/system/settings", &settings))
		stale.mustAPI(t, http.MethodPut, "/1.0/system/settings", settings)

		want := fmt.Sprint([]string{strings.Repeat("ef", 32)})
		for _, m := range c.members {
			require.Equal(t, want, fingerprints(m), m.name)
		}

		security["trusted_tls_client_cert_fingerprints"] = previous
		leader.mustAPI(t, http.MethodPut, "/1.0/system/security", security)
	})

	t.Run("concurrent shared config changes through different members", func(t *testing.T) {
		leader := c.leader(t)
		a, b := c.others(leader)[0], c.others(leader)[1]

		scriptlet := func(m *member) string {
			var settings map[string]any
			require.NoError(t, m.mustGet(t, "/1.0/system/settings", &settings))

			return fmt.Sprint(settings["server_registration_scriptlet"])
		}

		// put changes the scriptlet through both members at the same time and returns the error codes.
		put := func(etag string, scriptletA string, scriptletB string) []int {
			codes := make([]int, 2)
			start := time.Now()

			var wg sync.WaitGroup
			for i, change := range []struct {
				m         *member
				scriptlet string
			}{{a, scriptletA}, {b, scriptletB}} {
				// The current settings are read first, so the local ones stay unchanged.
				var settings map[string]any
				require.NoError(t, change.m.mustGet(t, "/1.0/system/settings", &settings))
				settings["server_registration_scriptlet"] = change.scriptlet

				wg.Go(func() {
					resp, _, err := change.m.tryAPIWithETag(http.MethodPut, "/1.0/system/settings", settings, etag)
					assert.NoError(t, err)
					codes[i] = resp.Code
				})
			}

			wg.Wait()

			// Neither change waits for the other one to be reloaded.
			require.Less(t, time.Since(start), 20*time.Second)

			return codes
		}

		converged := func(want ...string) {
			c.waitFor(t, "the members to agree on the scriptlet", func() bool {
				got := scriptlet(leader)
				for _, m := range c.members {
					if scriptlet(m) != got {
						return false
					}
				}

				return slices.Contains(want, got)
			})
		}

		previous := scriptlet(leader)
		scriptletA := "def server_registration(candidate):\n  # a\n  pass\n"
		scriptletB := "def server_registration(candidate):\n  # b\n  pass\n"

		// With the same ETag, only one of the changes is applied.
		_, etag, err := a.tryAPIWithETag(http.MethodGet, "/1.0/system/settings", nil, "")
		require.NoError(t, err)
		require.NotEmpty(t, etag)

		_, etagB, err := b.tryAPIWithETag(http.MethodGet, "/1.0/system/settings", nil, "")
		require.NoError(t, err)
		require.Equal(t, etag, etagB, "expect every member to report the same ETag")

		codes := put(etag, scriptletA, scriptletB)
		slices.Sort(codes)
		require.Equal(t, []int{0, http.StatusPreconditionFailed}, codes)
		converged(scriptletA, scriptletB)

		// The ETag of the replaced config is refused.
		resp, _, err := a.tryAPIWithETag(http.MethodPut, "/1.0/system/settings", map[string]any{"server_registration_scriptlet": previous}, etag)
		require.NoError(t, err)
		require.Equal(t, http.StatusPreconditionFailed, resp.Code)

		// Without an ETag, both are applied and the members agree on the last one.
		require.Equal(t, []int{0, 0}, put("", scriptletA, scriptletB))
		converged(scriptletA, scriptletB)

		var settings map[string]any
		require.NoError(t, leader.mustGet(t, "/1.0/system/settings", &settings))
		settings["server_registration_scriptlet"] = previous
		leader.mustAPI(t, http.MethodPut, "/1.0/system/settings", settings)
	})

	t.Run("the offline threshold is shared by the members", func(t *testing.T) {
		via := c.others(c.leader(t))[0]

		threshold := func(m *member) int {
			var cluster apisystem.Cluster
			require.NoError(t, m.mustGet(t, "/1.0/system/cluster", &cluster))

			return cluster.OfflineThresholdSeconds
		}

		previous := threshold(via)
		t.Cleanup(func() {
			via.mustAPI(t, http.MethodPut, "/1.0/system/cluster", apisystem.ClusterPut{OfflineThresholdSeconds: previous})
		})

		via.mustAPI(t, http.MethodPut, "/1.0/system/cluster", apisystem.ClusterPut{OfflineThresholdSeconds: previous + 10})

		for _, m := range c.members {
			require.Equal(t, previous+10, threshold(m), m.name)
		}
	})

	t.Run("a member down during a certificate renewal adopts the certificate on startup", func(t *testing.T) {
		leader := c.leader(t)
		down := c.others(leader)[1]

		certPEM, keyPEM := leader.readFile(t, "cluster.crt"), leader.readFile(t, "cluster.key")
		renewedPEM := renewClusterCertificate(t, certPEM, keyPEM)
		otherCertPEM, otherKeyPEM := newKeyPairPEM(t)

		down.kill()
		c.waitFor(t, down.name+" to be offline", func() bool {
			for _, member := range leader.members(t) {
				if member.Name == down.name {
					return member.Offline
				}
			}

			return false
		})

		// A new key would cut the offline member off for good.
		resp := leader.api(t, http.MethodPost, "/1.0/system/certificate", map[string]string{"certificate": otherCertPEM, "key": otherKeyPEM})
		require.Equal(t, http.StatusBadRequest, resp.Code, resp.Error)
		require.Contains(t, resp.Error, "online")

		// A renewal for the same key reaches the online members.
		leader.mustAPI(t, http.MethodPost, "/1.0/system/certificate", map[string]string{"certificate": renewedPEM, "key": keyPEM})

		for _, m := range []*member{leader, c.others(leader)[0]} {
			require.Equal(t, renewedPEM, m.readFile(t, "cluster.crt"), m.name)
			require.Equal(t, renewedPEM, presentedCertificate(t, m.address), m.name)
		}

		// The member missing the renewal adopts the certificate from the leader when it starts.
		require.Equal(t, certPEM, down.readFile(t, "cluster.crt"))
		down.start(t)
		c.waitForOnline(t, down)

		require.Eventually(t, func() bool {
			return down.hasFile("cluster.crt") && down.readFile(t, "cluster.crt") == renewedPEM
		}, 60*time.Second, time.Second, "the member adopts the renewed certificate")
		require.Equal(t, renewedPEM, presentedCertificate(t, down.address))

		// The members keep reaching each other with the renewed certificate.
		var security map[string]any
		require.NoError(t, down.mustGet(t, "/1.0/system/security", &security))
		down.mustAPI(t, http.MethodPut, "/1.0/system/security", security)
	})

	t.Run("cluster member hosts are taken down one at a time", func(t *testing.T) {
		leader := c.leader(t)
		down := c.others(leader)[1]
		target := c.others(leader)[0]

		// The members are contacted directly, since their server records may not tell yet.
		require.NoError(t, down.cmd.Process.Signal(syscall.SIGSTOP))

		resp := leader.api(t, http.MethodPost, "/1.0/provisioning/servers/"+target.name+"/system/:reboot", nil)

		require.NoError(t, down.cmd.Process.Signal(syscall.SIGCONT))
		require.Contains(t, resp.Error, `could lose the quorum`)
		require.Contains(t, resp.Error, strconv.Quote(down.name))
		c.waitForOnline(t, down)

		// The address of a member is part of the cluster.
		resp = leader.api(t, http.MethodPut, "/1.0/provisioning/servers/"+target.name+"/system/network", map[string]any{})
		require.Contains(t, resp.Error, "can not be changed")
	})

	t.Run("a file target is only removed after moving its files", func(t *testing.T) {
		filePath, apiPath := uploadImage(t, c.members[0], "three", "20260303")
		fileTarget := c.fileTarget(t, filePath)
		target := c.others(fileTarget)[0]

		resp := fileTarget.api(t, http.MethodDelete, "/1.0/membership/members/"+fileTarget.name, nil)
		require.Equal(t, http.StatusBadRequest, resp.Code, resp.Error)
		require.Contains(t, resp.Error, "file target")

		target.mustAPI(t, http.MethodPost, "/1.0/membership/members/"+fileTarget.name+"/:move-files", map[string]string{"target": target.name})
		require.True(t, target.hasFile(filePath))
		require.False(t, fileTarget.hasFile(filePath))

		for _, member := range target.members(t) {
			if member.Name == fileTarget.name {
				require.Zero(t, member.FileSets)
			}
		}

		// A new version is stored by the file target of the image, not by the member receiving it.
		var third *member
		for _, m := range c.others(fileTarget) {
			if m != target {
				third = m
			}
		}

		newFilePath, newAPIPath := uploadImage(t, third, "three", "20260304")
		require.Equal(t, target, c.fileTarget(t, newFilePath))

		for _, m := range c.members {
			status, _ := m.request(t, http.MethodGet, newAPIPath, nil, "")
			require.Equal(t, http.StatusOK, status, m.name)
		}

		target.mustAPI(t, http.MethodDelete, "/1.0/membership/members/"+fileTarget.name, nil)
		c.members = c.others(fileTarget)

		status, _ := target.request(t, http.MethodGet, "/1.0/provisioning/servers/"+fileTarget.name, nil, "")
		require.Equal(t, http.StatusNotFound, status, "server record of the removed member")

		// The removed member does not keep the client keypair of the cluster.
		require.Eventually(t, func() bool {
			return fileTarget.clientCertificate(t) != target.clientCertificate(t)
		}, 30*time.Second, 500*time.Millisecond)

		// The removed member starts again as a standalone server.
		require.Eventually(t, func() bool {
			members, err := fileTarget.tryMembers()
			return err == nil && len(members) == 0
		}, 60*time.Second, 500*time.Millisecond)

		fileTarget.kill()

		for _, m := range c.members {
			c.waitForDatabase(t, m)

			status, _ := m.request(t, http.MethodGet, apiPath, nil, "")
			require.Equal(t, http.StatusOK, status, m.name)
		}
	})

	t.Run("a forced removal loses the files of the member", func(t *testing.T) {
		filePath, apiPath := uploadImage(t, c.members[0], "four", "20260404")
		fileTarget := c.fileTarget(t, filePath)
		other := c.others(fileTarget)[0]

		rows := other.queryRows(t, fmt.Sprintf("SELECT id FROM cluster_members WHERE name = '%s'", fileTarget.name))
		require.Len(t, rows, 1)
		memberID := rows[0][0]

		memberRecords := func() map[string]int {
			records := map[string]int{}
			for _, table := range []string{"updates", "incus_images", "cluster_artifacts", "media"} {
				records[table] = len(other.queryRows(t, fmt.Sprintf("SELECT 1 FROM %s WHERE member_id = %v", table, memberID)))
			}

			records["config"] = len(other.queryRows(t, fmt.Sprintf("SELECT 1 FROM config WHERE node_id = %v", memberID)))

			return records
		}

		require.Positive(t, memberRecords()["incus_images"], "image record of the member")

		other.mustAPI(t, http.MethodDelete, "/1.0/membership/members/"+fileTarget.name+"?force=1", nil)
		c.members = c.others(fileTarget)
		fileTarget.kill()
		c.waitForDatabase(t, other)

		status, _ := other.request(t, http.MethodGet, apiPath, nil, "")
		require.Equal(t, http.StatusNotFound, status)

		status, _ = other.request(t, http.MethodGet, "/1.0/provisioning/servers/"+fileTarget.name, nil, "")
		require.Equal(t, http.StatusNotFound, status, "server record of the removed member")

		// The foreign keys delete the records of the removed member.
		for table, count := range memberRecords() {
			require.Zero(t, count, "%s records of the removed member", table)
		}
	})
}

func TestClusterHandover(t *testing.T) {
	c := setupClusterOf(t, 4)

	t.Run("a stopped voter hands over its role to the stand-by member", func(t *testing.T) {
		leader := c.leader(t)

		var leaving *member
		for _, member := range leader.members(t) {
			if member.Name != leader.name && slices.Equal(member.Roles, []string{"voter"}) {
				leaving = c.byName(t, member.Name)
				break
			}
		}

		require.NotNil(t, leaving)
		offset := leaving.logSize()

		leaving.stop()

		log := readFrom(filepath.Join(leaving.dir, "daemon.log"), offset)
		require.Contains(t, log, "Daemon shutdown completed successfully")
		require.NotContains(t, log, "Failed to hand over")

		// The leader lists the new roles once its local copy of the raft nodes caught up.
		c.waitFor(t, "the stand-by member to be promoted", func() bool {
			members, err := leader.tryMembers()

			return err == nil && voters(members) == 3
		})

		for _, member := range leader.members(t) {
			if member.Name == leaving.name {
				require.NotContains(t, member.Roles, "voter", "the stopped member is demoted")
			}
		}
	})

	t.Run("an unreachable member is removed without leaving anything behind", func(t *testing.T) {
		var down *member
		for _, m := range c.members {
			if m.cmd == nil {
				down = m
			}
		}

		require.NotNil(t, down, "the member stopped by the previous test")

		up := c.others(down)[0]
		up.mustAPI(t, http.MethodDelete, "/1.0/membership/members/"+down.name, nil)
		c.members = c.others(down)

		for _, member := range up.members(t) {
			require.NotEqual(t, down.name, member.Name)
		}

		status, _ := up.request(t, http.MethodGet, "/1.0/provisioning/servers/"+down.name, nil, "")
		require.Equal(t, http.StatusNotFound, status, "server record of the removed member")
	})
}

// roles returns the number of members of each role among members.
func roles(members []api.ClusterMember) map[string]int {
	counts := map[string]int{}
	for _, member := range members {
		for _, role := range member.Roles {
			counts[role]++
		}
	}

	return counts
}

// waitForRoles waits until all members list the members of the cluster with the given number of members of each role.
func (c *testCluster) waitForRoles(t *testing.T, want map[string]int) {
	t.Helper()

	c.waitFor(t, fmt.Sprintf("the roles %v", want), func() bool {
		for _, m := range c.members {
			members, err := m.tryMembers()
			if err != nil || len(members) != len(c.members) || !maps.Equal(roles(members), want) {
				return false
			}
		}

		return true
	})
}

// fileSets returns the number of file sets only stored by m, as listed by via.
func fileSets(t *testing.T, via *member, m *member) int {
	t.Helper()

	for _, member := range via.members(t) {
		if member.Name == m.name {
			return member.FileSets
		}
	}

	t.Fatalf("No member %q", m.name)

	return 0
}

// remove removes m through via, after moving its files to target, and waits for m to be a standalone server again.
func (c *testCluster) remove(t *testing.T, via *member, m *member, target *member) {
	t.Helper()

	c.removeRequest(t, via, m, target)
	c.waitRemoved(t, m)
}

// removeRequest removes m through via, after moving its files to target.
func (c *testCluster) removeRequest(t *testing.T, via *member, m *member, target *member) {
	t.Helper()

	if fileSets(t, via, m) > 0 {
		via.mustAPI(t, http.MethodPost, "/1.0/membership/members/"+m.name+"/:move-files", map[string]string{"target": target.name})
	}

	via.mustAPI(t, http.MethodDelete, "/1.0/membership/members/"+m.name, nil)
	c.members = c.others(m)
}

// waitRemoved waits for the removed member m to be a standalone server, which the other members no longer list.
func (c *testCluster) waitRemoved(t *testing.T, m *member) {
	t.Helper()

	// The removed member is reset and starts again as a standalone server.
	c.waitFor(t, m.name+" to be a standalone server", func() bool {
		members, err := m.tryMembers()
		return err == nil && len(members) == 0
	})

	for _, other := range c.members {
		c.waitForDatabase(t, other)

		for _, member := range other.members(t) {
			require.NotEqual(t, m.name, member.Name, other.name)
		}

		status, _ := other.request(t, http.MethodGet, "/1.0/provisioning/servers/"+m.name, nil, "")
		require.Equal(t, http.StatusNotFound, status, "server record of the removed member on %s", other.name)
	}
}

func TestClusterMembership(t *testing.T) {
	c := setupClusterOf(t, 5)

	binary, ip, basePort := testEnvironment(t)
	joiner := newMember(t, binary, c.root, "m6", net.JoinHostPort(ip, strconv.Itoa(basePort+5)))
	t.Cleanup(joiner.kill)

	t.Run("members beyond three voters are stand-by members", func(t *testing.T) {
		c.waitForRoles(t, map[string]int{"voter": 3, "stand-by": 2})
	})

	t.Run("a malformed join token is refused as invalid", func(t *testing.T) {
		joiner.start(t)

		resp := joiner.api(t, http.MethodPost, "/1.0/membership/:join", map[string]string{
			"server_name":     joiner.name,
			"cluster_address": joiner.address,
			"token":           "invalid",
		})
		require.Equal(t, http.StatusBadRequest, resp.Code, resp.Error)
		require.Contains(t, resp.Error, "Invalid join token")

		members, err := joiner.tryMembers()
		require.NoError(t, err)
		require.Empty(t, members, "the joiner stays standalone")
	})

	t.Run("a server managing servers joins by force only, which discards them", func(t *testing.T) {
		joiner.mustAPI(t, http.MethodPost, "/1.0/provisioning/servers", map[string]string{"name": "kept", "channel": "stable"})

		// The check comes before the token is looked at.
		resp := joiner.api(t, http.MethodPost, "/1.0/membership/:join", map[string]string{
			"server_name":     joiner.name,
			"cluster_address": joiner.address,
			"token":           "invalid",
		})
		require.Equal(t, http.StatusBadRequest, resp.Code, resp.Error)
		require.Contains(t, resp.Error, "manages 1 servers and 0 clusters")

		status, _ := joiner.request(t, http.MethodGet, "/1.0/provisioning/servers/kept", nil, "")
		require.Equal(t, http.StatusOK, status, "the refused join keeps the data")

		c.joinWith(t, c.leader(t), joiner, true)
		c.waitForRoles(t, map[string]int{"voter": 3, "stand-by": 3})

		for _, m := range c.members {
			status, _ := m.request(t, http.MethodGet, "/1.0/provisioning/servers/kept", nil, "")
			require.Equal(t, http.StatusNotFound, status, "%s: the forced join discards the data of the joiner", m.name)
		}
	})

	t.Run("a member joining a cluster with three voters is a stand-by member", func(t *testing.T) {
		for _, member := range c.members[0].members(t) {
			if member.Name == joiner.name {
				require.Equal(t, []string{"stand-by"}, member.Roles)
			}
		}
	})

	t.Run("the files of all stores move to another member and are served by all members", func(t *testing.T) {
		leader := c.leader(t)

		imagePath, imageAPIPath := uploadImage(t, c.others(leader)[0], "move", "20260505")
		source := c.fileTarget(t, imagePath)

		var target *member
		for _, m := range c.others(source) {
			if m != leader {
				target = m
				break
			}
		}

		const updateUUID = "5a0c2f8e-9b4d-4c1e-8f3a-2d7e6b5c4a39"
		updatePath := "updates/" + updateUUID + "/x86_64/update.raw"
		source.writeFile(t, updatePath, "update\n")
		leader.query(t, `INSERT INTO updates (uuid, origin, version, published_at, severity, upstream_channels, files, status, member_id) VALUES ('`+updateUUID+`', 'test', '1', CURRENT_TIMESTAMP, 'none', '[]', '[{"filename":"x86_64/update.raw","size":7}]', 'ready', (SELECT id FROM cluster_members WHERE name = '`+source.name+`'))`)

		const clusterUUID = "9e3b5d1a-7c2f-4b8e-a6d4-3f1c0e9b8a72"
		artifactPath := "artifacts/" + clusterUUID + "/terraform-configuration/main.tf"
		source.writeFile(t, artifactPath, "tf\n")
		leader.query(t, `INSERT INTO clusters (name, connection_url, status, last_updated, channel_id, uuid) VALUES ('move', 'https://192.0.2.1:8443', 'pending', CURRENT_TIMESTAMP, 1, '`+clusterUUID+`')`)
		leader.query(t, `INSERT INTO cluster_artifacts (cluster_id, name, description, properties, files, last_updated, member_id) VALUES ((SELECT id FROM clusters WHERE name = 'move'), 'terraform-configuration', '', '{}', '[{"Name":"main.tf","MimeType":"text/plain","Size":3}]', CURRENT_TIMESTAMP, (SELECT id FROM cluster_members WHERE name = '`+source.name+`'))`)

		require.Equal(t, 3, fileSets(t, leader, source))
		targetFileSets := fileSets(t, leader, target)

		leader.mustAPI(t, http.MethodPost, "/1.0/membership/members/"+source.name+"/:move-files", map[string]string{"target": target.name})

		for _, filePath := range []string{imagePath, updatePath, artifactPath} {
			require.True(t, target.hasFile(filePath), filePath)
			require.False(t, source.hasFile(filePath), filePath)
		}

		require.Zero(t, fileSets(t, leader, source))
		require.Equal(t, targetFileSets+3, fileSets(t, leader, target))

		files := map[string]string{
			imageAPIPath: "",
			"/1.0/provisioning/updates/" + updateUUID + "/files/x86_64/update.raw":      "update\n",
			"/1.0/provisioning/clusters/move/artifacts/terraform-configuration/main.tf": "tf\n",
		}

		for _, m := range c.members {
			for apiPath, want := range files {
				status, content := m.request(t, http.MethodGet, apiPath, nil, "")
				require.Equal(t, http.StatusOK, status, "%s %s", m.name, apiPath)

				if want != "" {
					require.Equal(t, want, string(content), "%s %s", m.name, apiPath)
				}
			}
		}
	})

	t.Run("files are not moved to the same member, an unknown member or an offline member", func(t *testing.T) {
		leader := c.leader(t)
		others := c.others(leader)
		source := others[0]

		resp := leader.api(t, http.MethodPost, "/1.0/membership/members/"+source.name+"/:move-files", map[string]string{"target": source.name})
		require.Equal(t, http.StatusBadRequest, resp.Code, resp.Error)

		resp = leader.api(t, http.MethodPost, "/1.0/membership/members/"+source.name+"/:move-files", map[string]string{"target": "unknown"})
		require.Equal(t, http.StatusNotFound, resp.Code, resp.Error)

		offline := others[1]
		offline.kill()

		c.waitFor(t, offline.name+" to be offline", func() bool {
			for _, member := range leader.members(t) {
				if member.Name == offline.name {
					return member.Offline
				}
			}

			return false
		})

		resp = leader.api(t, http.MethodPost, "/1.0/membership/members/"+source.name+"/:move-files", map[string]string{"target": offline.name})
		require.Equal(t, http.StatusServiceUnavailable, resp.Code, resp.Error)
		require.Contains(t, resp.Error, "is offline")

		offline.start(t)
		c.waitForAllOnline(t)
		c.waitForRoles(t, map[string]int{"voter": 3, "stand-by": 3})
	})

	var removed *member

	t.Run("removing a voter promotes a stand-by member", func(t *testing.T) {
		leader := c.leader(t)

		for _, member := range leader.members(t) {
			if !member.Leader && slices.Equal(member.Roles, []string{"voter"}) {
				removed = c.byName(t, member.Name)
				break
			}
		}

		require.NotNil(t, removed)

		c.remove(t, leader, removed, leader)
		c.waitForRoles(t, map[string]int{"voter": 3, "stand-by": 2})
	})

	t.Run("a removed member joins again", func(t *testing.T) {
		require.NotNil(t, removed, "the member removed by the previous test")

		c.join(t, c.leader(t), removed)
		c.waitForRoles(t, map[string]int{"voter": 3, "stand-by": 3})

		for _, m := range c.members {
			status, _ := m.request(t, http.MethodGet, "/1.0/provisioning/servers/"+removed.name, nil, "")
			require.Equal(t, http.StatusOK, status, "server record of the member joined again on %s", m.name)
		}
	})

	t.Run("a leader removing itself hands over the leadership", func(t *testing.T) {
		leader := c.leader(t)
		others := c.others(leader)

		// The task status of a member is answered through any member.
		cert := others[0].serverCertificate(t)
		status, err := others[0].tryTaskStatus(&cert, leader.name)
		require.NoError(t, err)
		require.Equal(t, leader.address, status.Address)
		require.NotEmpty(t, busyTasks(status), "the leader runs the leader tasks")

		// The new leader starts no leader task, while the former leader still runs or stops it.
		stopWatching := watchLeaderTasks(t, leader, others)
		c.removeRequest(t, leader, leader, others[0])
		require.Empty(t, stopWatching(), "leader tasks busy on the former and the new leader at the same time")

		c.waitRemoved(t, leader)
		leader.kill()

		c.waitForRoles(t, map[string]int{"voter": 3, "stand-by": 2})

		newLeader := c.leader(t)
		require.NotEqual(t, leader.name, newLeader.name)

		c.waitFor(t, "the new leader to run all leader tasks", func() bool {
			status, err := newLeader.tryTaskStatus(&cert, "")
			return err == nil && len(busyTasks(status)) == len(status.Tasks)
		})
	})

	t.Run("a cluster of two members has a single voter", func(t *testing.T) {
		for len(c.members) > 2 {
			leader := c.leader(t)
			m := c.others(leader)[0]

			c.remove(t, leader, m, leader)
			m.kill()
		}

		c.waitForRoles(t, map[string]int{"voter": 1, "spare": 1})
	})

	var lastRemoved *member

	t.Run("removing the leader of a cluster of two members leaves the other member as the voter", func(t *testing.T) {
		leader := c.leader(t)
		remaining := c.others(leader)[0]

		// The remaining member forwards the removal to the leader, which removes itself.
		c.remove(t, remaining, leader, remaining)
		leader.kill()
		lastRemoved = leader

		c.waitForRoles(t, map[string]int{"voter": 1})
		require.Equal(t, remaining.name, c.leader(t).name)

		remaining.mustAPI(t, http.MethodPut, "/1.0/provisioning/channels/stable", map[string]string{"description": "changed by the only member"})
	})

	t.Run("a member, which died during its reset, completes the reset on startup", func(t *testing.T) {
		require.NotNil(t, lastRemoved, "the member removed by the previous test")
		m := lastRemoved

		// The reset marks itself first, so a member dying right after the marker still holds everything of the cluster.
		m.writeFile(t, "reset.pending", "")
		m.writeFile(t, "cluster.crt", "stale certificate")
		m.writeFile(t, "updates/stale/x86_64/update.raw", "stale update")
		clientCertificate := m.clientCertificate(t)

		m.start(t)

		require.Eventually(t, func() bool {
			members, err := m.tryMembers()
			return err == nil && len(members) == 0
		}, 60*time.Second, 500*time.Millisecond, "the member starts as a standalone server")

		require.False(t, m.hasFile("reset.pending"))
		require.False(t, m.hasFile("cluster.crt"))
		require.False(t, m.hasFile("updates/stale"))
		require.NotEqual(t, clientCertificate, m.clientCertificate(t), "the client keypair of the cluster is replaced")

		m.kill()
	})
}

// busyTasks returns the names of the leader tasks running or stopping in status.
func busyTasks(status *membershipapi.InternalClusterTaskStatus) []string {
	var busy []string
	for _, t := range status.Tasks {
		if t.State != membershipapi.TaskStopped {
			busy = append(busy, t.Name)
		}
	}

	return busy
}

// watchLeaderTasks watches the leader tasks of the former leader and the other members until the returned function is called.
// The function returns the overlaps seen: a task busy on another member, while the former leader still had it busy.
func watchLeaderTasks(t *testing.T, former *member, others []*member) func() []string {
	t.Helper()

	cert := former.serverCertificate(t)
	done := make(chan struct{})
	result := make(chan []string, 1)

	go func() {
		var overlaps []string

		for {
			select {
			case <-done:
				result <- overlaps
				return

			case <-time.After(100 * time.Millisecond):
			}

			// The former leader only goes from busy to stopped, so a task busy before and after the other members were asked was busy in between.
			before, err := former.tryTaskStatus(&cert, "")
			if err != nil {
				continue
			}

			othersBusy := map[string][]string{}
			for _, other := range others {
				status, err := other.tryTaskStatus(&cert, "")
				if err != nil {
					continue
				}

				for _, name := range busyTasks(status) {
					othersBusy[name] = append(othersBusy[name], other.name)
				}
			}

			after, err := former.tryTaskStatus(&cert, "")
			if err != nil {
				continue
			}

			for _, name := range busyTasks(before) {
				if len(othersBusy[name]) > 0 && slices.Contains(busyTasks(after), name) {
					overlaps = append(overlaps, fmt.Sprintf("%s on %s and %v", name, former.name, othersBusy[name]))
				}
			}
		}
	}()

	return func() []string {
		close(done)
		return <-result
	}
}
