package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// serverBMCPowerOffAndOn powers the given server off and on again through its
// BMC, which is provided by an in-process Redfish proxy.
func serverBMCPowerOffAndOn(serverName string) func(t *testing.T, tmpDir string) {
	return func(t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t)
		defer stop()

		// The Redfish proxy acts as the BMC of the IncusOS instance.
		endpoint := startRedfishProxy(t, serverName)

		// Register cleanup
		t.Cleanup(serverPowerStateCleanup(t, serverName))
		t.Cleanup(serverBMCConfigCleanup(t, tmpDir, serverName))

		// Setup
		t.Log("Configure the BMC of the server")
		mustSetServerBMCConfig(t, tmpDir, serverName, "redfish-v1-generic", endpoint)

		assertBMCConfig(t, serverName, endpoint)

		// Run test
		//
		// Powering off the server with --force is the equivalent of pulling the
		// plug, so the file systems of the server need to be flushed before,
		// otherwise recently written files are lost.
		t.Log("Flush the file systems of the server")
		mustRunWithTimeout(t, `incus exec %s -- sync`, time.Minute, serverName)

		t.Log("Power off the server via BMC")
		mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning server bmc server-power-off %s --force`, 5*time.Minute, cpuArch, serverName)

		// Assertions
		assertInstanceStatus(t, serverName, "Stopped", 3*time.Minute)
		assertBMCServerPowerState(t, serverName, "Off", 2*time.Minute)

		// Run test
		t.Log("Power on the server via BMC")
		mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning server bmc server-power-on %s`, 5*time.Minute, cpuArch, serverName)

		// Assertions
		assertInstanceStatus(t, serverName, "Running", 2*time.Minute)
		assertBMCServerPowerState(t, serverName, "On", 2*time.Minute)

		mustWaitIncusOSReady(t, []string{serverName})
		mustWaitInventoryReady(t, []string{serverName})
	}
}

// assertBMCConfig verifies, that the BMC config has been applied, that the
// certificate presented by the BMC has been pinned and that Operations Center
// is able to collect data from the BMC.
func assertBMCConfig(t *testing.T, serverName string, endpoint string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.bmc_config.endpoint'`, cpuArch, serverName)
	require.Equal(t, endpoint, resp.OutputTrimmed(), "expect the BMC endpoint to be persisted")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.bmc_config.certificate'`, cpuArch, serverName)
	require.Contains(t, resp.Output(), "-----BEGIN CERTIFICATE-----", "expect the BMC certificate to be pinned")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r '.bmc_config.auto_pin_certificate'`, cpuArch, serverName)
	require.Equal(t, "false", resp.OutputTrimmed(), "expect auto pinning of the BMC certificate to be disabled after pinning")

	// A refresh collects the BMC data synchronously.
	mustRunWithTimeout(t, `../bin/operations-center.linux.%s provisioning server bmc refresh %s`, time.Minute, cpuArch, serverName)

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.bmc_data.bmc_protocol'`, cpuArch, serverName)
	require.Equal(t, "Redfish", resp.OutputTrimmed(), "expect BMC data to be collected")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.bmc_data.server_power_state'`, cpuArch, serverName)
	require.Equal(t, "On", resp.OutputTrimmed(), "expect the server to be reported as powered on")
}

// assertInstanceStatus waits for the Incus instance to reach the wanted status.
func assertInstanceStatus(t *testing.T, name string, status string, timeout time.Duration) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	desc := fmt.Sprintf("instance %q to be %q", name, status)

	success, err := waitForSuccessWithTimeout(t, desc, `incus list -f json | jq -r -e '[ .[] | select(.name == "%s" and .status == "%s") ] | length == 1'`, timeout, name, status)
	require.NoErrorf(t, err, "expect %s", desc)

	if !success {
		fmt.Println("====[ Instances ]====")
		resp := mustRun(t, `incus list -f json | jq -c '[ .[] | { name: .name, status: .status } ]'`)
		fmt.Println(resp.Output())
	}

	require.Truef(t, success, "expect %s", desc)
}

// assertBMCServerPowerState waits for Operations Center to report the wanted
// power state of the server.
func assertBMCServerPowerState(t *testing.T, serverName string, powerState string, timeout time.Duration) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	desc := fmt.Sprintf("BMC power state of server %q to be %q", serverName, powerState)

	success, err := waitForSuccessWithTimeout(t, desc, `../bin/operations-center.linux.%s provisioning server show %s -f json | jq -r -e '.bmc_data.server_power_state == "%s"'`, timeout, cpuArch, serverName, powerState)
	require.NoErrorf(t, err, "expect %s", desc)

	if !success {
		fmt.Println("====[ BMC Data ]====")
		resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s --bmc-data`, cpuArch, serverName)
		fmt.Println(resp.Output())
	}

	require.Truef(t, success, "expect %s", desc)
}
