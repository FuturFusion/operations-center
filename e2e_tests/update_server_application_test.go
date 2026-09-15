package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// updateServerApplication triggers the update of a single application of a standalone server.
func updateServerApplication(name string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t)
		defer stop()

		// Register cleanup
		t.Cleanup(prodChannelCleanup(t))

		t.Log("Update application - assign most recent update to prod channel")
		newestUpdateUUIDResp := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(.update_status == "ready") ] | sort_by(.version) | reverse | first | .uuid'`, cpuArch)
		mustRun(t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel stable,prod`, cpuArch, newestUpdateUUIDResp.OutputTrimmed())

		// The newly assigned update is only picked up with the next poll of the server.
		ok, err := waitForSuccessWithTimeout(ctx, t, "an application to need an update",
			`../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '[ .[] | select(.name == "%s") | .version_data.applications[] | select(.needs_update == true) ] | length >= 1'`,
			10*time.Minute, cpuArch, name)
		require.NoError(t, err)
		if !ok {
			printServerList(t)
			t.Skip("No installed application is outdated, the update assigned to the channel provides no newer application")
		}

		// Pick any outdated application, the two updates may differ in any of them.
		applicationResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '[ .[] | select(.name == "%s") | .version_data.applications[] | select(.needs_update == true) ] | first | .name'`, cpuArch, name)
		application := applicationResp.OutputTrimmed()

		expectedVersionResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.applications[] | select(.name == "%s") | .available_version'`, cpuArch, name, application)
		expectedVersion := expectedVersionResp.OutputTrimmed()

		osVersionResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.os.version'`, cpuArch, name)
		osVersion := osVersionResp.OutputTrimmed()

		t.Logf("Update application - trigger update of application %q to version %q", application, expectedVersion)
		mustRun(t, `../bin/operations-center.linux.%s provisioning server system update %s --application %s`, cpuArch, name, application)

		stopUpdate := timeTrack(t, "application update")
		defer stopUpdate()

		// A failed update is not reported by IncusOS, so it only surfaces as a timeout.
		ok, err = waitForSuccessWithTimeout(ctx, t, "the application update to complete",
			`../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "%s") | .server_status == "ready" and .server_status_detail == "" and ([ .version_data.applications[] | select(.name == "%s" and .version == "%s") ] | length == 1)'`,
			15*time.Minute, cpuArch, name, application, expectedVersion)
		require.NoError(t, err)
		if !ok {
			printServerList(t)
			logVMDebugInfo(t, name)
		}

		require.Truef(t, ok, "Update application: %q did not reach version %q", application, expectedVersion)

		// An update of a single application must not touch the OS.
		resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.os.version'`, cpuArch, name)
		require.Equal(t, osVersion, resp.OutputTrimmed(), "Update application: the version of the OS has changed")

		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.os.version_next'`, cpuArch, name)
		require.Empty(t, resp.OutputTrimmed(), "Update application: an OS update has been staged")

		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.os.needs_reboot'`, cpuArch, name)
		require.Equal(t, "false", resp.OutputTrimmed(), "Update application: a reboot is required")

		// The OS is still on the update it has been installed from.
		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.os.needs_update // false'`, cpuArch, name)
		require.Equal(t, "true", resp.OutputTrimmed(), "Update application: the OS does not need an update anymore")

		t.Log("Update application - update completed")
	}
}
