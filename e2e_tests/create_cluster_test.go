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

func createCluster() func(t *testing.T, tmpDir string) {
	return createClusterWithChannelName("stable")
}

func createClusterWithChannelName(channelName string) func(t *testing.T, tmpDir string) {
	return func(t *testing.T, tmpDir string) {
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
		mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --channel %s --services-config %s --application-seed-config %s`, cpuArch, instanceIPs[0], servers, channelName, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

		// Assertions
		assertIncusRemote(t, "incus-os-cluster", instanceIPs[0])
		assertInventory(t, "incus-os-cluster")
		assertTerraformArtifact(t, "incus-os-cluster")
		assertWebsocketEventsInventoryUpdate(t, "incus-os-cluster")
	}
}

func clusterCleanup(t *testing.T) func() {
	t.Helper()

	return func() {
		if noCleanup {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(30*time.Second))
		defer cancel()

		stop := timeTrack(t, "cluster cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[].name'`, cpuArch)
		if !resp.Success() {
			t.Error(resp.Error())
		} else {
			for cluster := range strings.Lines(resp.Output()) {
				cluster = strings.TrimSpace(cluster)
				resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning cluster remove %s --force`, cpuArch, cluster)
				if !resp.Success() {
					t.Error(resp.Error())
				}
			}
		}
	}
}
