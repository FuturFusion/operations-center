package e2e

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func factoryResetCluster(ctx context.Context, t *testing.T, tmpDir string) {
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

	// Without a token, Operations Center creates a token for the re-registration of
	// the servers. It removes the token, once every server has used it.
	factoryResetTokensCmd := `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.description == "Factory reset of cluster \"incus-os-cluster\"") ] | length'`
	factoryResetTokensBefore, err := strconv.Atoi(mustRun(t, factoryResetTokensCmd, cpuArch).OutputTrimmed())
	require.NoError(t, err)

	t.Log("Factory reset cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset incus-os-cluster`, cpuArch)

	assertClusterFactoryReset(t, "incus-os-cluster", names)

	factoryResetTokens := mustRun(t, factoryResetTokensCmd, cpuArch).OutputTrimmed()
	require.Equal(t, strconv.Itoa(factoryResetTokensBefore+1), factoryResetTokens, "expect the factory reset to create a token for the re-registration")

	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(ctx, t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	mustWaitInventoryReady(ctx, t, instanceNames)

	factoryResetTokens = mustRun(t, factoryResetTokensCmd, cpuArch).OutputTrimmed()
	require.Equal(t, strconv.Itoa(factoryResetTokensBefore), factoryResetTokens, "expect the token of the factory reset to be removed after the re-registration of all the servers")

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
	assertIncusRemote(t, "incus-os-cluster-after-factory-reset", names)
	assertInventory(t, "incus-os-cluster-after-factory-reset", names)
}
