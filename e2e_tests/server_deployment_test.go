package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	deployTokenSeedName = "incus-os-deploy"

	// Config key and value set by the BIOS profile handed in with the deployment.
	deployBIOSProfileName        = "incus-redfish-proxy"
	deployBIOSProfileConfigKey   = "user.e2e.bios-profile"
	deployBIOSProfileConfigValue = "applied"
)

// setupEmptyInstance creates an Incus instance without any operating system,
// which is the equivalent of a server as it is delivered.
func setupEmptyInstance(name string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t)
		defer stop()

		t.Cleanup(serverDeploymentCleanup(t, name, removeDeployMedia))
		t.Cleanup(cleanupIncusOS(t, []string{name}))
		t.Cleanup(serverDeploymentCleanup(t, name, removeDeployedServer))

		removeDeployedServer(ctx, t, name)

		status := mustInstanceStatus(ctx, t, name)
		if status != "" {
			t.Logf("%s is in status %q, removing it in order to deploy it from scratch", name, status)
			require.NoError(t, removeInstanceWithContext(ctx, t, name))
		}

		removeDeployMedia(ctx, t, name)

		// Secure boot is enabled from the start, since Incus resets the UEFI
		// variables of an instance, when the setting changes.
		mustRun(t, `incus init --empty --vm %s -c security.secureboot=true -c limits.cpu=%s -c limits.memory=%s -d root,size=%s -d root,io.cache=unsafe`, name, cpuCount, memorySize, diskSize)
		mustRun(t, `incus config device add %s vtpm tpm`, name)
		mustRun(t, `incus config set %s systemd.credential.fully-enable-incus-agent=true`, name)
	}
}

// removeDeployedServer removes the server from Operations Center.
func removeDeployedServer(ctx context.Context, t *testing.T, name string) {
	t.Helper()

	resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.name == "%s") ] | length == 1'`, cpuArch, name)
	if !resp.Success() {
		return
	}

	resp = runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server deploy-cancel %s --skip-cleanup`, cpuArch, name)
	if resp.Success() {
		waitCtx, cancel := context.WithTimeout(ctx, strechedTimeout(time.Minute))
		resp = runWithContext(waitCtx, t, `../bin/operations-center.linux.%s provisioning server deploy-status %s --wait`, cpuArch, name)
		cancel()

		t.Logf("Cancelled the deployment of server %q: %s", name, resp.OutputTrimmed())
	}

	resp = runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server remove %s`, cpuArch, name)
	if !resp.Success() {
		t.Logf("Failed to remove server %q: %s", name, resp.Error())
	}
}

// removeDeployMedia removes the installation media the Redfish proxy has
// imported for the instance.
func removeDeployMedia(ctx context.Context, t *testing.T, name string) {
	t.Helper()

	volume := name + "-boot-media.iso"

	resp := runWithContext(ctx, t, `incus storage volume list default -f json | jq -r -e '[ .[] | select(.name == "%s") ] | length == 1'`, volume)
	if !resp.Success() {
		return
	}

	err := retryStorageCmdWithContext(ctx, t, fmt.Sprintf("delete the storage volume %q", volume), `incus storage volume delete default %s`, volume)
	if err != nil {
		t.Logf("Failed to remove the installation media of %q: %v", name, err)
	}
}

func serverDeploymentCleanup(t *testing.T, name string, cleanup func(ctx context.Context, t *testing.T, name string)) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(2*time.Minute))
		defer cancel()

		cleanup(ctx, t, name)
	}
}

// serverDeployment pre-registers the empty instance as a server and has
// Operations Center deploy IncusOS on it through the BMC.
func serverDeployment(name string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t)
		defer stop()

		endpoint := startRedfishProxy(t, name)

		// Setup
		token := createProvisioningToken(t)

		t.Cleanup(cleanupTokenSeed(t, token, deployTokenSeedName))

		createDeployTokenSeed(t, tmpDir, token)

		biosProfileFilename := filepath.Join(tmpDir, "bios_profile.yaml")

		err := os.WriteFile(biosProfileFilename, redfishProxyBIOSProfileYAMLTemplate, 0o600)
		require.NoError(t, err)

		// Run test
		t.Log("Pre-register the server")
		mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning server pre-register %s --bmc-api-type redfish-v1-generic --bmc-endpoint %s --bmc-auto-pin-certificate --bmc-username e2e --bmc-password e2e`, time.Minute, cpuArch, name, endpoint)

		// Assertions
		assertServerPreRegistered(t, name, endpoint)

		// Run test
		//
		// The BIOS profile is only handed in with the deployment, it is not part
		// of the profiles shipped with Operations Center.
		t.Log("Deploy the server")

		resp := runWithTimeout(t, `../bin/operations-center.linux.%s provisioning server deploy %s %s %s --bios-profiles %s --wait`, 35*time.Minute, cpuArch, name, token, deployTokenSeedName, biosProfileFilename)
		if !resp.Success() {
			fmt.Println("====[ Deployment status ]====")
			fmt.Println(run(t, `../bin/operations-center.linux.%s provisioning server deploy-status %s -f yaml`, cpuArch, name).Output())
			fmt.Println("====[ Instance ]====")
			fmt.Println(run(t, `incus config show %s`, name).Output())
		}

		require.NoError(t, fmtRunErr(resp), "expect the deployment of the server to complete")

		// Assertions
		assertServerDeployed(t, name)

		mustWaitIncusOSReady(ctx, t, []string{name})
		mustWaitInventoryReady(ctx, t, []string{name})

		printServerList(t)
	}
}

// createDeployTokenSeed adds the public token seed the server is deployed from.
func createDeployTokenSeed(t *testing.T, tmpDir string, token string) {
	t.Helper()

	seedFilename := filepath.Join(tmpDir, "incusos_deploy_seed.yaml")

	err := os.WriteFile(seedFilename, deployIncusOSSeedFileYAMLTemplate, 0o600)
	require.NoError(t, err)

	resp := run(t, `../bin/operations-center.linux.%s provisioning token seed remove %s %s`, cpuArch, token, deployTokenSeedName)
	if resp.Success() {
		t.Logf("Removed left over token seed %q of token %q", deployTokenSeedName, token)
	}

	mustRun(t, `../bin/operations-center.linux.%s provisioning token seed add %s %s %s --public --description "E2E test server deployment"`, cpuArch, token, deployTokenSeedName, seedFilename)
}

// assertServerPreRegistered verifies, that the server is known to Operations
// Center, that the data of its BMC has been collected and that the BIOS
// profiles shipped with Operations Center do not cover it.
func assertServerPreRegistered(t *testing.T, name string, endpoint string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.server_status'`, cpuArch, name)
	require.Equal(t, "unregistered", resp.OutputTrimmed(), "expect the server to be pre-registered")

	// Refreshes the BMC data, which is otherwise collected in the background.
	assertBMCConfig(t, name, endpoint, "Off")

	// The deployment addresses the virtual media device by its ID.
	mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.bmc_data.virtual_media | has("manager:CD")'`, cpuArch, name)

	instanceUUID := mustRun(t, `incus config get %s volatile.uuid`, name).OutputTrimmed()
	require.NotEmpty(t, instanceUUID, "expect the instance to have a UUID")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.bmc_data.system_uuid'`, cpuArch, name)
	require.Equal(t, strings.ToLower(instanceUUID), strings.ToLower(resp.OutputTrimmed()), "expect the BMC to report the UUID of the instance")

	resp = run(t, `../bin/operations-center.linux.%s provisioning server bios-profile %s`, cpuArch, name)
	require.NoError(t, resp.err)
	require.False(t, resp.Success(), "expect none of the BIOS profiles of Operations Center to match the server")
	require.Contains(t, resp.Output(), "No BIOS profile matches", "expect none of the BIOS profiles of Operations Center to match the server")
}

// assertServerDeployed verifies, that the deployment has completed with the
// BIOS profile handed in and that it left the instance in the expected state.
func assertServerDeployed(t *testing.T, name string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server deploy-status %s -f json | jq -r -e '.state'`, cpuArch, name)
	require.Equal(t, "completed", resp.OutputTrimmed(), "expect the deployment to be completed")

	mustRun(t, `../bin/operations-center.linux.%s provisioning server deploy-status %s -f json | jq -r -e '.bios_profiles == ["%s"]'`, cpuArch, name, deployBIOSProfileName)

	resp = mustRun(t, `incus config get %s %s`, name, deployBIOSProfileConfigKey)
	require.Equal(t, deployBIOSProfileConfigValue, resp.OutputTrimmed(), "expect the BIOS attribute of the profile to be applied")

	resp = mustRun(t, `incus config get %s security.secureboot`, name)
	require.Equal(t, "true", resp.OutputTrimmed(), "expect secure boot to be enabled")

	resp = mustRun(t, `incus config device list %s`, name)
	require.NotContains(t, resp.Output(), "boot-media", "expect the installation media to be detached")

	// The installed server has taken over the pre-registered one.
	mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.server_type == "incus") ] | length == 1'`, cpuArch)
}
