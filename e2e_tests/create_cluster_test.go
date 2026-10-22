package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createCluster(serverNames []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return createClusterWithChannelName("stable", serverNames)
}

func createClusterAndAddServerAndRemoveServer() func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		createClusterWithChannelName("stable", []string{"IncusOS01", "IncusOS02", "IncusOS03"})(ctx, t, tmpDir)

		printServerList(t)

		// Update LVM settings.
		err := os.WriteFile(filepath.Join(tmpDir, "lvm_enable.yaml"), incusOSServiceLVMConfig, 0o600)
		require.NoError(t, err)

		mustRun(t, `../bin/operations-center.linux.%s provisioning server os service edit IncusOS04:lvm < %s`, cpuArch, filepath.Join(tmpDir, "lvm_enable.yaml"))

		// Add IncusOS04 to cluster.
		mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add-servers incus-os-cluster --server-names IncusOS04`, cpuArch)

		// Assertions
		assertClusterMembers(t, "incus-os-cluster", []string{"IncusOS01", "IncusOS02", "IncusOS03", "IncusOS04"})

		// Evacuate IncusOS03.
		mustRun(t, `../bin/operations-center.linux.%s provisioning server system evacuate IncusOS03`, cpuArch)

		t.Log("Wait for server to be evacuated")
		ok, err := waitForSuccessWithTimeout(ctx, t, "server evacuated", `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "IncusOS03") | .version_data.in_maintenance == 2'`, 2*time.Minute, cpuArch)
		require.NoError(t, err, "expect IncusOS03 to be evacuated")
		if !ok {
			fmt.Println("====[ Instance List ]====")
			resp := mustRun(t, "../bin/operations-center.linux.%s inventory instance list", cpuArch)
			fmt.Println(resp.Output())
			t.FailNow()
		}

		mustDeleteClusterImages(t, "incus-os-cluster")

		// Remove IncusOS03 from cluster.
		mustRun(t, `../bin/operations-center.linux.%s provisioning cluster remove-servers incus-os-cluster --server-names IncusOS03`, cpuArch)

		// Assertions
		assertClusterMembers(t, "incus-os-cluster", []string{"IncusOS01", "IncusOS02", "IncusOS04"})
		assertRemovedServerToReappear(ctx, t)
	}
}

func createClusterAndForceRemoveLostServer() func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		createClusterWithChannelName("stable", []string{"IncusOS01", "IncusOS02", "IncusOS03"})(ctx, t, tmpDir)

		printServerList(t)

		// Lose IncusOS03, it is neither evacuated nor reachable from now on.
		err := stopInstanceWithContext(ctx, t, "IncusOS03")
		require.NoError(t, err, "expect IncusOS03 to be stopped")

		// Remove the lost IncusOS03 from the cluster.
		mustRun(t, `../bin/operations-center.linux.%s provisioning cluster remove-servers incus-os-cluster --server-names IncusOS03 --force`, cpuArch)

		// Assertions
		assertClusterMembers(t, "incus-os-cluster", []string{"IncusOS01", "IncusOS02"})
		assertServerDetachedFromCluster(t, "IncusOS03")

		// The kept server record can now be removed from operations center.
		mustRun(t, `../bin/operations-center.linux.%s provisioning server remove IncusOS03`, cpuArch)

		assertServerGone(t, "IncusOS03")
	}
}

func createClusterFromManualUpload(serverNames []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		createClusterWithChannelName("manual", serverNames)(ctx, t, tmpDir)

		assertServersRunManualUpdate(t, serverNames)
	}
}

func createClusterWithChannelName(channelName string, serverNames []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		// Register cleanup
		t.Cleanup(clusterCleanup(t))

		createAndAssertCluster(channelName, serverNames)(ctx, t, tmpDir)
	}
}

// createAndAssertCluster keeps the cluster, so the next scenario of a suite can
// use it.
func createAndAssertCluster(channelName string, serverNames []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t, "create cluster with channel name")
		defer stop()

		// Pre check
		mustNotBeAlreadyClustered(t)

		// Run test
		mustCreateCluster(t, tmpDir, channelName, serverNames)

		// Assertions
		assertIncusRemote(t, incusOSClusterName, serverNames)
		assertInventory(t, incusOSClusterName, serverNames)
		assertTerraformArtifact(t, incusOSClusterName)
		assertWebsocketEventsInventoryUpdate(ctx, t, incusOSClusterName)
	}
}
