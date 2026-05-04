package e2e

import (
	"context"
	"net"
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

	clientCertificate := getClientCertificate(t)

	err = os.WriteFile(
		filepath.Join(tmpDir, "application.yaml"),
		replacePlaceholders(
			incusOSClusterApplicationConfig,
			map[string]string{
				"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 6)),
			},
		),
		0o600,
	)
	require.NoError(t, err)

	names := []string{"IncusOS01", "IncusOS02", "IncusOS03"}

	instanceIPs, _ := mustGetInstanceIPAndNames(t, names)

	servers := strings.Join(names, " --server-names ")

	// Run test
	t.Log("Create cluster")
	clusterName := "incus-os-cluster"
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add %s https://%s --server-names %s --channel prod --services-config %s --application-seed-config %s`, cpuArch, clusterName, net.JoinHostPort(instanceIPs[0], "8443"), servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

	// Assertions
	assertIncusRemote(t, clusterName, names)
	assertInventory(t, clusterName, names)
	assertTerraformArtifact(t, clusterName)
	assertWebsocketEventsInventoryUpdate(t, clusterName)

	t.Log("Start some small VMs for the cluster to have some minimal workload.")
	for i := range names {
		mustRun(t, `incus launch --vm images:alpine/edge %s:mini-alpine-%d -c limits.cpu=1 -c limits.memory=256MiB -c security.secureboot=false -c migration.stateful=true`, clusterName, i)
	}

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

	t.Log("Update post_restore_delay")
	mustRun(t, `EDITOR='sed -i "s/    post_restore_delay:.*/    post_restore_delay: 20s/"' script -q -c '../bin/operations-center.linux.%s provisioning cluster edit incus-os-cluster' /dev/null`, cpuArch)

	t.Log("Update cluster - trigger update")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster update incus-os-cluster --reboot`, cpuArch)

	ctx, cancel := context.WithTimeout(t.Context(), strechedTimeout(30*time.Minute))
	defer cancel()

	previousUpdateStatusDescription := ""

	for {
		resp := mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | .update_status.in_progress_status.in_progress'`, cpuArch, clusterName)
		if resp.OutputTrimmed() == "" {
			break
		}

		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | .update_status.in_progress_status.status_description // ""'`, cpuArch, clusterName)
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

		case <-time.After(1 * time.Second):
		}
	}

	t.Log("Update cluster - update completed")
}

func prodChannelCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
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
			return
		}

		for updateUUID := range strings.Lines(resp.Output()) {
			updateUUID = strings.TrimSpace(updateUUID)
			resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel stable`, cpuArch, updateUUID)
			if !resp.Success() {
				t.Error(resp.Error())
			}
		}
	}
}
