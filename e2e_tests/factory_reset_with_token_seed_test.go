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

func factoryResetClusterWithTokenSeed(ctx context.Context, t *testing.T, tmpDir string) {
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

	t.Log("Create token seed for factory reset")
	token := createProvisioningToken(t)

	const tokenSeedName = "incus-os-cluster-factory-reset"

	seedFilename := filepath.Join(tmpDir, "incusos_factory_reset_seed.yaml")

	err = os.WriteFile(seedFilename, incusOSFactoryResetSeedFileYAMLTemplate, 0o600)
	require.NoError(t, err)

	t.Cleanup(cleanupTokenSeed(t, token, tokenSeedName))
	createTokenSeed(t, token, tokenSeedName, seedFilename)

	tokenUsesCmd := `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '.[] | select(.uuid == %q) | .uses_remaining'`
	tokenUsesBefore, err := strconv.Atoi(mustRun(t, tokenUsesCmd, cpuArch, token).OutputTrimmed())
	require.NoError(t, err)

	t.Log("Factory reset cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset incus-os-cluster %s %s`, cpuArch, token, tokenSeedName)

	assertClusterFactoryReset(t, "incus-os-cluster", names)

	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(ctx, t, []string{"IncusOS01", "IncusOS02", "IncusOS03"})

	mustWaitInventoryReady(ctx, t, instanceNames)

	tokenUses := mustRun(t, tokenUsesCmd, cpuArch, token).OutputTrimmed()
	require.Equal(t, strconv.Itoa(tokenUsesBefore-len(names)), tokenUses, "expect every server to use the given token for the re-registration")

	// The timezone is only set by the token seed.
	assertServersTimezone(ctx, t, instanceNames, "Europe/Zurich")

	err = os.WriteFile(
		filepath.Join(tmpDir, "application-post-factory-reset.yaml"),
		replacePlaceholders(
			incusOSClusterApplicationConfigPostFactoryResetWithTokenSeed,
			map[string]string{
				"$CLIENT_CERTIFICATE$": indent(clientCertificate, strings.Repeat(" ", 6)),
			},
		),
		0o600,
	)
	require.NoError(t, err)

	// Post factory reset, the servers register with their machine ID again.
	servers = strings.Join(instanceNames, " --server-names ")

	t.Log("Create cluster incus-os-cluster-after-factory-reset")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster add incus-os-cluster-after-factory-reset https://%s --server-names %s --services-config %s --application-seed-config %s`, cpuArch, net.JoinHostPort(instanceIPs[0], "8443"), servers, filepath.Join(tmpDir, "services.yaml"), filepath.Join(tmpDir, "application-post-factory-reset.yaml"))

	// Assertions
	assertIncusRemote(t, "incus-os-cluster-after-factory-reset", names)
	assertInventory(t, "incus-os-cluster-after-factory-reset", names)
}
