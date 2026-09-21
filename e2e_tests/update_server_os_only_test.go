package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// updateServerOSOnly triggers an update of the OS of a standalone server, which
// is restricted to the OS and leaves the installed applications alone.
func updateServerOSOnly(name string) func(ctx context.Context, t *testing.T, tmpDir string) {
	return func(ctx context.Context, t *testing.T, tmpDir string) {
		t.Helper()

		stop := timeTrack(t)
		defer stop()

		// Register cleanup
		t.Cleanup(prodChannelCleanup(t))

		t.Log("Update OS only - assign most recent update to prod channel")
		newestUpdateUUIDResp := mustRun(t, `../bin/operations-center.linux.%s provisioning update list -f json | jq -r '[ .[] | select(.update_status == "ready") ] | sort_by(.version) | reverse | first | .uuid'`, cpuArch)
		mustRun(t, `../bin/operations-center.linux.%s provisioning update assign-channels %s --channel stable,prod`, cpuArch, newestUpdateUUIDResp.OutputTrimmed())

		// The newly assigned update is only picked up with the next poll of the server.
		ok, err := waitForSuccessWithTimeout(ctx, t, "the OS to need an update",
			`../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "%s") | .version_data.os.needs_update // false'`,
			10*time.Minute, cpuArch, name)
		require.NoError(t, err)
		if !ok {
			printServerList(t)
			t.Skip("The OS is not outdated, the update assigned to the channel provides no newer OS")
		}

		expectedVersionResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.os.available_version'`, cpuArch, name)
		expectedVersion := expectedVersionResp.OutputTrimmed()

		// Remember the applications, so it can be verified none of them moved.
		applicationsResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -c '[ .[] | select(.name == "%s") | .version_data.applications[] | {name, version} ] | sort_by(.name)'`, cpuArch, name)
		applications := applicationsResp.OutputTrimmed()

		// An application, which needs an update, is what makes this test meaningful:
		// a full OS update would carry it along, an OS only update must not.
		outdatedResp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '[ .[] | select(.name == "%s") | .version_data.applications[] | select(.needs_update == true) ] | length'`, cpuArch, name)
		if outdatedResp.OutputTrimmed() == "0" {
			printServerList(t)
			t.Skip("No installed application is outdated, an OS only update can not be told apart from a full one")
		}

		t.Logf("Update OS only - trigger update of the OS to version %q", expectedVersion)
		mustRun(t, `../bin/operations-center.linux.%s provisioning server system update %s --os --os-only`, cpuArch, name)

		stopUpdate := timeTrack(t, "OS only update")
		defer stopUpdate()

		// The OS update is staged, so it surfaces as version_next rather than as
		// version, and the server asks for a reboot to apply it. A failed update is
		// not reported by IncusOS, so it only surfaces as a timeout.
		ok, err = waitForSuccessWithTimeout(ctx, t, "the OS only update to complete",
			`../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "%s") | .server_status == "ready" and .server_status_detail == "" and .version_data.os.version_next == "%s"'`,
			15*time.Minute, cpuArch, name, expectedVersion)
		require.NoError(t, err)
		if !ok {
			printServerList(t)
			logVMDebugInfo(t, name)
		}

		require.Truef(t, ok, "Update OS only: the OS did not stage version %q", expectedVersion)

		// The point of the test: none of the applications has been touched.
		resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -c '[ .[] | select(.name == "%s") | .version_data.applications[] | {name, version} ] | sort_by(.name)'`, cpuArch, name)
		require.Equal(t, applications, resp.OutputTrimmed(), "Update OS only: the version of an application has changed")

		// The staged OS update still asks for a reboot to be applied.
		resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r '.[] | select(.name == "%s") | .version_data.os.needs_reboot'`, cpuArch, name)
		require.Equal(t, "true", resp.OutputTrimmed(), "Update OS only: no reboot is required to apply the staged update")

		t.Log("Update OS only - update completed")
	}
}
