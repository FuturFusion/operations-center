package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// factoryResetClusterWithTokenSeed leaves the servers registered with their
// hostname. The next scenario of the suite reuses them.
func factoryResetClusterWithTokenSeed(names []string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t, "factory reset cluster with token seed")
		defer stop()

		// Setup
		_, instanceNames := mustGetInstanceIPAndNames(t, names)

		t.Log("Create token seed for factory reset")
		token := createProvisioningToken(t)

		const tokenSeedName = "incus-os-cluster-factory-reset"

		seedFilename := filepath.Join(tmpDir, "incusos_factory_reset_seed.yaml")

		err := os.WriteFile(seedFilename, incusOSFactoryResetSeedFileYAMLTemplate, 0o600)
		require.NoError(t, err)

		// Register cleanup
		t.Cleanup(cleanupTokenSeed(t, token, tokenSeedName))

		createTokenSeed(t, token, tokenSeedName, seedFilename)

		tokenUsesCmd := `../bin/operations-center.linux.%s provisioning token list -f json | jq -r '.[] | select(.uuid == %q) | .uses_remaining'`
		tokenUsesBefore, err := strconv.Atoi(mustRun(t, tokenUsesCmd, cpuArch, token).OutputTrimmed())
		require.NoError(t, err)

		// Run test
		t.Log("Factory reset cluster")
		mustRun(t, `../bin/operations-center.linux.%s provisioning cluster factory-reset %s %s %s`, cpuArch, incusOSClusterName, token, tokenSeedName)

		// Assertions
		assertClusterFactoryReset(t, incusOSClusterName, names)

		time.Sleep(strechedTimeout(10 * time.Second)) // Wait for the factory reset to happen.

		mustWaitIncusOSReady(ctx, t, names)

		// Post factory reset, the servers register with their hostname again.
		mustWaitInventoryReady(ctx, t, instanceNames)

		tokenUses := mustRun(t, tokenUsesCmd, cpuArch, token).OutputTrimmed()
		require.Equal(t, strconv.Itoa(tokenUsesBefore-len(names)), tokenUses, "expect every server to use the given token for the re-registration")

		// The timezone is only set by the token seed.
		assertServersTimezone(ctx, t, instanceNames, "Europe/Zurich")
	}
}
