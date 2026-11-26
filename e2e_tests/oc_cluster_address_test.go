package e2e

import (
	"context"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

// ocAddressPointsToOtherMember points the address of Operations Center, which
// the managed servers use, at another member. Every member presents the
// certificate of the cluster, so the servers trust any member.
func ocAddressPointsToOtherMember(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())
	changer, target, restorer := members[0], members[1], members[2]
	serverNames := []string{"IncusOS01", "IncusOS02", "IncusOS03"}

	original := mustOperationsCenterMemberSystemConfig(t, changer, "network")
	originalAddress, ok := original["address"].(string)
	require.True(t, ok, "expect an address in the network configuration")

	newAddress := "https://" + target.address
	require.NotEqual(t, originalAddress, newAddress, "expect the address to point at another member than %s", target.name)

	t.Cleanup(systemConfigRestoreCleanup(t, tmpDir, changer, "network", original))

	require.Equal(t, mustServedCertificate(ctx, t, changer.address), mustServedCertificate(ctx, t, target.address), "expect %s and %s to present the same certificate", changer.name, target.name)

	t.Logf("Point the address at %s through %s", target.name, changer.name)

	// The network configuration has a local part. So each member changes
	// its own configuration.
	changed := mustOperationsCenterMemberSystemConfig(t, changer, "network")
	changed["address"] = newAddress
	mustRun(t, `%s system network edit < %s`, changer.oc, mustWriteSystemConfig(t, tmpDir, changer, "network", changed))

	assertOperationsCenterAddress(ctx, t, members, serverNames, newAddress)

	t.Logf("Restore the address through %s", restorer.name)

	restored := mustOperationsCenterMemberSystemConfig(t, restorer, "network")
	restored["address"] = originalAddress
	mustRun(t, `%s system network edit < %s`, restorer.oc, mustWriteSystemConfig(t, tmpDir, restorer, "network", restored))

	assertOperationsCenterAddress(ctx, t, members, serverNames, originalAddress)
}

// assertOperationsCenterAddress asserts that all members use address and
// that the managed servers serverNames connect to it.
func assertOperationsCenterAddress(ctx context.Context, t *testing.T, members []operationsCenterMember, serverNames []string, address string) {
	t.Helper()

	for _, member := range members {
		require.Equal(t, address, mustOperationsCenterMemberSystemConfig(t, member, "network")["address"], "expect %s to use the address %s", member.name, address)
	}

	// Each member updates its own server record, when it reloads the shared
	// network configuration.
	reader := members[len(members)-1]
	ok, err := waitForSuccessWithTimeout(ctx, t, "public connection URL of the members", `%s provisioning server list -f json | jq -e '[ .[] | select(.server_type == "operations-center" and .public_connection_url == "%s") ] | length == %d'`, ocClusterReuseTimeout, reader.oc, address, len(members))
	require.NoError(t, err, "Failed to list the servers through %s", reader.name)
	require.True(t, ok, "expect the server records of all the members to carry the public connection URL %s", address)

	for _, name := range serverNames {
		resp := mustRun(t, `%s provisioning server os system provider show %s:`, reader.oc, name)

		var provider incusosapi.SystemProvider
		err := yaml.Unmarshal([]byte(resp.OutputTrimmed()), &provider)
		require.NoError(t, err, "Failed to decode the provider configuration of %s", name)

		require.Equal(t, address, provider.Config.Config["server_url"], "expect %s to connect to %s", name, address)
	}

	mustRun(t, `%s provisioning server list -f json | jq -e '[ .[] | select(.name as $n | %s | index($n)) | select(.server_status == "ready") ] | length == %d'`, reader.oc, asJSON(t, serverNames), len(serverNames))
}
