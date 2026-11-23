package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func basicOperationsCenterInteractions(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	assertOperationsCenterCliAdmin(t)
	assertOperationsCenterCliQuery(t)
	assertOperationsCenterCliSystem(t)
	assertOperationsCenterCliAdminDebugPprof(t, tmpDir)
	assertOperationsCenterMetrics(t)
	assertOperationsCenterCliProvisioningTokenSeed(t, tmpDir)
	assertOperationsCenterCliProvisioningClusterTemplate(t, tmpDir)
}

func basicOperationsCenterInteractionsUpdatesCleanupAndRefresh(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	assertOperationsCenterCliUpdateCleanupAndRefresh(ctx, t)
}

func registerServer(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	assertServerRegistrationScriptletEffects(t)
	assertEvacuateAndRestoreRejectedWithoutCluster(t, "IncusOS01")
}

// assertEvacuateAndRestoreRejectedWithoutCluster asserts that a server without
// a cluster cannot be evacuated or restored. Its status detail stays empty.
func assertEvacuateAndRestoreRejectedWithoutCluster(t *testing.T, name string) {
	t.Helper()

	for _, action := range []string{"evacuate", "restore"} {
		resp := run(t, `../bin/operations-center.linux.%s provisioning server system %s %s`, cpuArch, action, name)
		require.False(t, resp.Success(), "expect %s of %s without cluster to fail", action, name)
		require.Contains(t, resp.Output(), "is not a cluster member", "expect %s of %s to be rejected, because it is not a cluster member", action, name)
	}

	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "%s") | (.version_data.in_maintenance // 0) == 0 and .server_status_detail == ""'`, cpuArch, name)
	require.Equal(t, "true", resp.OutputTrimmed(), "expect %s to stay out of maintenance with an empty status detail", name)
}
