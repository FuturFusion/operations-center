package e2e

import (
	"net"
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

	instanceIPs, instanceNames := mustGetInstanceIPAndNames(t, names)

	servers := strings.Join(names, " --server-names ")

	// Run test
	t.Log("Create cluster incus-os-cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s --server-names %s --services-config %s --application-seed-config %s`, cpuArch, net.JoinHostPort(instanceIPs[0], "8443"), servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

	t.Log("Factory reset cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset incus-os-cluster`, cpuArch)
	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	mustWaitInventoryReady(t, instanceNames)

	// Post factory reset, the servers register with their machine ID again.
	servers = strings.Join(instanceNames, " --server-names ")

	err = os.WriteFile(
		filepath.Join(tmpDir, "application-post-factory-reset.yaml"),
		replacePlaceholders(
			incusOSClusterApplicationConfigPostFactoryReset,
			map[string]string{
				"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 6)),
			},
		),
		0o600,
	)
	require.NoError(t, err)

	t.Log("Create cluster incus-os-cluster-after-factory-reset")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster-after-factory-reset https://%s --server-names %s --services-config %s --application-seed-config %s`, cpuArch, net.JoinHostPort(instanceIPs[0], "8443"), servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application-post-factory-reset.yaml"))

	// Assertions
	assertIncusRemote(t, "incus-os-cluster-after-factory-reset")
	assertInventory(t, "incus-os-cluster-after-factory-reset", names)
}
