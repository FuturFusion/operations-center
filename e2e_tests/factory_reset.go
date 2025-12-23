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
	t.Log("Create cluster incus-os-cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster https://%s:8443 --server-names %s --services-config %s --application-seed-config %s`, cpuArch, instanceIPs[0], servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application.yaml"))

	t.Log("Factory reset cluster")
	// mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset incus-os-cluster %s factory-reset`, cpuArch, token)
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset incus-os-cluster`, cpuArch)
	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	mustWaitInventoryReady(t, instanceNames)

	clientCertificate := getClientCertificate(t)
	applicationConfig := replacePlaceholders(incusOSClusterApplicationConfigPostFactoryReset,
		map[string]string{
			"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 10)),
		},
	)

	err = os.WriteFile(filepath.Join(tmpDir, "application-post-factory-reset.yaml"), applicationConfig, 0o600)
	require.NoError(t, err)

	t.Log("Create cluster incus-os-cluster-after-factory-reset-1")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster-after-factory-reset-1 https://%s:8443 --server-names %s --services-config %s --application-seed-config %s`, cpuArch, instanceIPs[0], servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application-post-factory-reset.yaml"))

	// Assertions
	assertIncusRemote(t, "incus-os-cluster-after-factory-reset-1", instanceIPs[0])
	assertInventory(t, "incus-os-cluster-after-factory-reset-1")

	// Run test with token seed
	t.Log("Create token seed for factory reset")
	token := createProvisioningToken(t)

	incusOSSeedFileYAML := replacePlaceholders(incusOSSeedFileYAMLTemplate,
		map[string]string{
			"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 10)),
		},
	)

	err = os.WriteFile(filepath.Join(tmpDir, "incusos_seed.yaml"), incusOSSeedFileYAML, 0o600)
	require.NoError(t, err)

	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s factory-reset %s/incusos_seed.yaml`, cpuArch, token, tmpDir)

	t.Log("Factory reset cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset incus-os-cluster-after-factory-reset-1 %s factory-reset`, cpuArch, token)
	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	mustWaitInventoryReady(t, instanceNames)

	t.Log("Create cluster incus-os-cluster-after-factory-reset-2")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster-after-factory-reset-2 https://%s:8443 --server-names %s --services-config %s --application-seed-config %s`, cpuArch, instanceIPs[0], servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application-post-factory-reset.yaml"))

	// Assertions
	assertIncusRemote(t, "incus-os-cluster-after-factory-reset-2", instanceIPs[0])
	assertInventory(t, "incus-os-cluster-after-factory-reset-2")
}
