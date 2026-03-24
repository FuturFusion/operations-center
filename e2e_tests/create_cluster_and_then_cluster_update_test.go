package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TODO: combine with createCluster to remove redundant code. createCluster needs
// to accept the channel as argument.
func createClusterAndThenClusterUpdate(t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Pre check
	mustNotBeAlreadyClustered(t)

	// Register cleanup
	t.Cleanup(clusterCleanup(t))

	// Setup
	err := os.WriteFile(filepath.Join(tmpDir, "services.yaml"), incusOSClusterServicesConfig, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "application.yaml"), incusOSClusterApplicationConfig, 0o600)
	require.NoError(t, err)

	instanceIPs, instanceNames := mustGetInstanceIPAndNames(t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	servers := strings.Join(instanceNames, " --server-names ")

	// Run test
	t.Log("Create cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --channel prod --services-config %s --application-seed-config %s`, cpuArch, instanceIPs[0], servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

	// Assertions
	assertIncusRemote(t, "incus-os-cluster", instanceIPs[0])
	assertInventory(t, "incus-os-cluster")
	assertTerraformArtifact(t, "incus-os-cluster")
	assertWebsocketEventsInventoryUpdate(t, "incus-os-cluster")

	t.Cleanup(prodChannelCleanup(t))

	t.Log("Update cluster - pre update state")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list`, cpuArch)
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster show incus-os-cluster`, cpuArch)

	t.Log("Update cluster - assign most recent update to prod channel")
	newestUpdateUUIDResp := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(.update_status == "ready") ] | sort_by(.version) | reverse | first | .uuid'`, cpuArch)
	mustRun(t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel stable,prod`, cpuArch, newestUpdateUUIDResp.OutputTrimmed())

	t.Log("Update cluster - list pending updates")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list`, cpuArch)
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster show incus-os-cluster`, cpuArch)

	stopUpdate := timeTrack(t, "cluster update")
	defer stopUpdate()

	t.Log("Update cluster - trigger update")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster update incus-os-cluster --reboot`, cpuArch)

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(20*time.Minute))
	defer cancel()

	previousUpdateStatusDescription := ""

	for {
		resp := mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "incus-os-cluster") | .update_status.in_progress_status.in_progress'`, cpuArch)
		if resp.OutputTrimmed() == "" {
			break
		}

		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "incus-os-cluster") | .update_status.in_progress_status.status_description // ""'`, cpuArch)
		statusDescription := resp.OutputTrimmed()

		if statusDescription != previousUpdateStatusDescription {
			t.Logf("Update cluster: %s", statusDescription)
		}

		previousUpdateStatusDescription = statusDescription

		if debug {
			resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq '[ .[] | { "server_status": .server_status, "server_status_detail": .server_status_detail, "version_data": .version_data } ]'`, cpuArch)
			debugf("per server status: %s", resp.Output())
		}

		select {
		case <-ctx.Done():
			t.Fatalf("Update deadline reached: %v", ctx.Err())
			return

		case <-time.After(10 * time.Second):
		}
	}

	t.Log("Update cluster - update completed")
}

func prodChannelCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "prod channel cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '.[] | select(.channels | index("prod")) | .uuid'`, cpuArch)
		if !resp.Success() {
			t.Error(resp.Error())
		} else {
			for updateUUID := range strings.Lines(resp.Output()) {
				updateUUID = strings.TrimSpace(updateUUID)
				resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel stable`, cpuArch, updateUUID)
				if !resp.Success() {
					t.Error(resp.Error())
				}
			}
		}
	}
}
