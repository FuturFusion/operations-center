package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func factoryResetCluster(t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Pre check
	mustNotBeAlreadyClustered(t)

	// Setup
	err := os.WriteFile(filepath.Join(tmpDir, "services.yaml"), incusOSClusterServicesConfig, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "application.yaml"), incusOSClusterApplicationConfig, 0o600)
	require.NoError(t, err)

	instanceIPs, instanceNames := mustGetInstanceIPAndNames(t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	servers := strings.Join(instanceNames, " --server-names ")

	// Run test
	t.Log("Create cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --services-config %s --application-seed-config %s`, cpuArch, instanceIPs[0], servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

	t.Log("Factory reset cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster remove --mode factory-reset incus-os-cluster`, cpuArch)
	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	t.Log("Create cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --services-config %s --application-seed-config %s`, cpuArch, instanceIPs[0], servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

	// Assertions
	assertIncusRemote(t, instanceIPs[0])
	assertInventory(t)
	assertTerraformArtifact(t)
}
