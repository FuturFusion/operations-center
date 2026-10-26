package clustertests

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

// testCluster is an Operations Center cluster of three members.
type testCluster struct {
	members []*member
}

func setupCluster(t *testing.T) *testCluster {
	t.Helper()

	if os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST") == "" {
		t.Skip("Set OPERATIONS_CENTER_CLUSTER_TEST to run the cluster tests")
	}

	binary := os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_BINARY")
	if binary == "" {
		binary = "../bin/operations-centerd"
	}

	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	require.FileExists(t, binary, `Build the daemon with "make build" or set OPERATIONS_CENTER_CLUSTER_TEST_BINARY`)

	ip := os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_ADDRESS")
	if ip == "" {
		ip = globalUnicastIP(t)
	}

	basePort := 17443
	if os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_PORT") != "" {
		basePort, err = strconv.Atoi(os.Getenv("OPERATIONS_CENTER_CLUSTER_TEST_PORT"))
		require.NoError(t, err)
	}

	root, err := os.MkdirTemp("", "oc-cluster-")
	require.NoError(t, err)

	c := &testCluster{}
	for i := range 3 {
		address := net.JoinHostPort(ip, strconv.Itoa(basePort+i))
		c.members = append(c.members, newMember(t, binary, root, fmt.Sprintf("m%d", i+1), address))
	}

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

	offsets := map[*member]int64{}
	for _, m := range c.members[1:] {
		for _, other := range c.members {
			offsets[other] = other.logSize()
		}

		first.mustAPI(t, http.MethodPost, "/1.0/membership/tokens", map[string]string{
			"server_name": m.name,
			"expiry":      "1h",
		})

		var token string
		resp := first.mustAPI(t, http.MethodGet, "/1.0/membership/tokens/"+m.name, nil)
		err := resp.MetadataAsStruct(&token)
		require.NoError(t, err)

		m.start(t)
		m.mustAPI(t, http.MethodPost, "/1.0/membership/:join", map[string]string{
			"server_name":     m.name,
			"cluster_address": m.address,
			"token":           token,
		})

		c.waitForOnline(t, m)
	}

	// The members trust a new member once they got a heartbeat listing it.
	c.waitFor(t, "all members to trust each other", func() bool {
		for _, m := range c.members {
			if !strings.Contains(readFrom(filepath.Join(m.dir, "daemon.log"), offsets[m]), "Cluster member states changed, updating authentication") {
				return false
			}
		}

		return true
	})

	// A just promoted member does not take part in elections until it knows about it.
	c.waitFor(t, "all members to be voters", func() bool {
		for _, m := range c.members {
			members, err := m.tryMembers()
			if err != nil || len(members) != len(c.members) {
				return false
			}

			for _, member := range members {
				if !slices.Equal(member.Roles, []string{"voter"}) {
					return false
				}
			}
		}

		return true
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

// waitForDatabase waits until m can access the database, e.g. while a new leader is elected.
func (c *testCluster) waitForDatabase(t *testing.T, m *member) {
	t.Helper()

	c.waitFor(t, m.name+" to access the database", func() bool {
		_, err := m.tryMembers()
		return err == nil
	})
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

			var network apisystem.NetworkFull
			require.NoError(t, resp.MetadataAsStruct(&network))
			require.Equal(t, "https://"+c.members[0].address, network.OperationsCenterAddress, m.name)
		}
	})

	t.Run("a member stays reachable, while its https listener covers its cluster address", func(t *testing.T) {
		m := c.members[1]

		setRestServerAddress := func(address string) {
			resp := m.mustAPI(t, http.MethodGet, "/1.0/system/network", nil)

			var network apisystem.NetworkFull
			require.NoError(t, resp.MetadataAsStruct(&network))

			network.RestServerAddress = address
			m.mustAPI(t, http.MethodPut, "/1.0/system/network", apisystem.NetworkFullPut{
				NetworkPut:       network.NetworkPut,
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

	t.Run("only cluster members manage the membership of a member", func(t *testing.T) {
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

			for _, path := range []string{"/internal/membership/:reset", "/internal/membership/:rebalance"} {
				status, body = m.httpsRequest(t, &trusted, http.MethodPost, path, nil, nil)
				require.Equal(t, http.StatusForbidden, status, "%s %s: %s", m.name, path, body)
			}
		}

		security["trusted_tls_client_cert_fingerprints"] = previous
		leader.mustAPI(t, http.MethodPut, "/1.0/system/security", security)
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
		c.waitForOnline(t, paused)
		leader.mustAPI(t, http.MethodPut, "/1.0/system/security", security)

		for _, m := range c.members {
			require.Equal(t, fmt.Sprint([]string{strings.Repeat("cd", 32)}), fingerprints(m), m.name)
		}
	})

	t.Run("secure boot media is served by any member", func(t *testing.T) {
		leader := c.leader(t)
		holder := c.others(leader)[0]

		holder.writeFile(t, "secure-boot-media/abcdefghijkl.iso", "secure boot enrollment media")

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

		other.mustAPI(t, http.MethodDelete, "/1.0/membership/members/"+fileTarget.name+"?force=1", nil)
		c.members = c.others(fileTarget)
		fileTarget.kill()
		c.waitForDatabase(t, other)

		status, _ := other.request(t, http.MethodGet, apiPath, nil, "")
		require.Equal(t, http.StatusNotFound, status)

		status, _ = other.request(t, http.MethodGet, "/1.0/provisioning/servers/"+fileTarget.name, nil, "")
		require.Equal(t, http.StatusNotFound, status, "server record of the removed member")
	})
}
