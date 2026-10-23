package e2e

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// factoryResetCluster leaves the servers registered with their hostname. The
// next scenario of the suite reuses them.
func factoryResetCluster(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	// Setup
	names := []string{"IncusOS01", "IncusOS02", "IncusOS03"}

	_, instanceNames := mustGetInstanceIPAndNames(t, names)

	// Without a token, Operations Center creates a token for the re-registration of
	// the servers. It removes the token, once every server has used it.
	factoryResetTokensCmd := `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '[ .[] | select(.description == "Factory reset of cluster \"incus-os-cluster\"") ] | length'`
	factoryResetTokensBefore, err := strconv.Atoi(mustRun(t, factoryResetTokensCmd, cpuArch).OutputTrimmed())
	require.NoError(t, err)

	// Run test
	t.Log("Factory reset cluster")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset %s`, cpuArch, incusOSClusterName)

	// Assertions
	assertClusterFactoryReset(t, incusOSClusterName, names)

	factoryResetTokens := mustRun(t, factoryResetTokensCmd, cpuArch).OutputTrimmed()
	require.Equal(t, strconv.Itoa(factoryResetTokensBefore+1), factoryResetTokens, "expect the factory reset to create a token for the re-registration")

	time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

	mustWaitIncusOSReady(ctx, t, names)

	// Post factory reset, the servers register with their hostname again.
	mustWaitInventoryReady(ctx, t, instanceNames)

	factoryResetTokens = mustRun(t, factoryResetTokensCmd, cpuArch).OutputTrimmed()
	require.Equal(t, strconv.Itoa(factoryResetTokensBefore), factoryResetTokens, "expect the token of the factory reset to be removed after the re-registration of all the servers")
}
