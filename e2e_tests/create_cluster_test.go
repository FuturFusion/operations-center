package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func createCluster(t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Pre check
	clusterListResp, err := run(t, "incus exec IncusOS01 -- incus cluster list")
	require.NoError(t, err)
	require.NotEqual(t, 0, clusterListResp.exitCode, "IncusOS01 is already part of a cluster")

	// Setup
	err = os.WriteFile(filepath.Join(tmpDir, "services.yaml"), incusOSClusterServicesConfig, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "application.yaml"), incusOSClusterApplicationConfig, 0o600)
	require.NoError(t, err)

	var firstInstanceIP string

	ipResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "IncusOS01") | .state.network | to_entries[] | .value.addresses[]? | select(.family == "inet" and .scope == "global") | .address' | head -n1`)
	firstInstanceIP = strings.TrimSpace(ipResp.Output())

	instanceNames := make([]string, 0, 3)
	for i := range 3 {
		instanceID := i + 1
		nameResp := mustRun(t, `incus list -f json | jq -r '.[] | select(.name == "IncusOS0%d") | .state.os_info.hostname'`, instanceID)
		instanceNames = append(instanceNames, strings.TrimSpace(nameResp.Output()))
	}

	servers := strings.Join(instanceNames, " --server-names ")

	// Run test
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --services-config %s --application-seed-config %s`, cpuArch, firstInstanceIP, servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

	// Assertions
	assertIncusRemote(t, firstInstanceIP)
	assertInventory(t)
	assertTerraformArtifact(t)
}
