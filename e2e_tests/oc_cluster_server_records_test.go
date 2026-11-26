package e2e

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

// ocClusterMemberServerRecords checks the server records of the members. Each
// member registers itself under its own name with its own certificate. The
// records of the members can not be renamed or deleted.
func ocClusterMemberServerRecords(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())

	address := mustOperationsCenterMemberSystemConfig(t, members[0], "network")["address"]
	channel := mustOperationsCenterMemberSystemConfig(t, members[0], "updates")["server_default_channel"]

	for i, member := range members {
		// Read the record through another member. All members share the records.
		reader := members[(i+1)%len(members)]

		t.Logf("Assert the server record of %s through %s", member.name, reader.name)

		record := mustOperationsCenterServerRecord(t, reader, member.name)

		require.Equal(t, api.ServerTypeOperationsCenter, record.Type, "expect %s to be a server of type operations-center", member.name)
		require.Equal(t, api.ServerStatusReady, record.Status, "expect %s to be ready", member.name)
		require.Equal(t, "https://"+member.address, record.ConnectionURL, "expect %s to be reached at its own address", member.name)
		require.Equal(t, address, record.PublicConnectionURL, "expect %s to carry the address of Operations Center", member.name)
		require.Equal(t, channel, record.Channel, "expect %s to follow the default server channel", member.name)

		// The record holds the certificate of the member, not the certificate
		// of the cluster, which every member presents.
		memberCertificate := mustRun(t, `incus exec %s -- cat %s/server.crt`, member.name, ocDataDir).OutputTrimmed()
		require.Equal(t, memberCertificate, strings.TrimSpace(record.Certificate), "expect the record of %s to hold the certificate of the member", member.name)

		block, _ := pem.Decode([]byte(record.Certificate))
		require.NotNil(t, block, "expect the record of %s to hold a PEM certificate", member.name)
		require.NotEqual(t, mustServedCertificate(ctx, t, member.address), block.Bytes, "expect the record of %s not to hold the certificate of the cluster", member.name)

		resp := run(t, `%s provisioning server rename %s %s-renamed`, reader.oc, member.name, member.name)
		require.False(t, resp.Success(), "expect the rename of the record of the member %s to be refused", member.name)

		resp = run(t, `%s provisioning server remove %s`, reader.oc, member.name)
		require.False(t, resp.Success(), "expect the removal of the record of the member %s to be refused", member.name)

		mustOperationsCenterServerRecord(t, reader, member.name)
	}

	// The record of a standalone Operations Center is renamed, when clustering
	// is enabled.
	mustRun(t, `%s provisioning server list -f json | jq -e '[ .[] | select(.name == "%s") ] | length == 0'`, members[0].oc, api.ServerNameOperationsCenter)
}

// mustOperationsCenterServerRecord returns the server record name, read
// through member.
func mustOperationsCenterServerRecord(t *testing.T, member operationsCenterMember, name string) api.Server {
	t.Helper()

	resp := mustRun(t, `%s provisioning server show %s -f json`, member.oc, name)

	var server api.Server
	err := json.Unmarshal(resp.output.Bytes(), &server)
	require.NoError(t, err, "Failed to decode the server record of %s", name)

	return server
}
