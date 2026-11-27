package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// manualEvacuateAndRestore evacuates and restores a single cluster member
// outside of a rolling update. On success, only polling clears the status
// detail.
func manualEvacuateAndRestore(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	clusterName := incusOSClusterName
	target := "IncusOS02"
	instanceName := "evacuate-c1"

	t.Cleanup(workloadCleanup(t, clusterName, []string{instanceName}))
	t.Cleanup(serverRestoreCleanup(t, target))

	t.Logf("Launch container %q on %s", instanceName, target)
	// Force migration. With the default "auto", Incus may stop the instance instead.
	mustRunWithTimeout(t, `incus launch images:alpine/edge %s:%s --target %s -c cluster.evacuate=migrate`, 5*time.Minute, clusterName, instanceName, target)
	assertInstanceLocation(ctx, t, "pre evacuate", clusterName, instanceName, target)

	// Incus accepts a restore of a member that is not evacuated. The operation
	// succeeds. The status detail must not stay "restoring". The settled state
	// equals the state before the restore. The wait is still meaningful, because
	// the restore call returns only after the service sets "restoring".
	t.Logf("Restore %s that is not evacuated", target)
	mustRun(t, `../bin/operations-center.linux.%s provisioning server system restore %s`, cpuArch, target)
	assertServerMaintenanceSettled(ctx, t, "restore not evacuated", target, 0, "up to date")
	assertInstanceLocation(ctx, t, "restore not evacuated", clusterName, instanceName, target)

	t.Logf("Evacuate %s", target)
	mustRun(t, `../bin/operations-center.linux.%s provisioning server system evacuate %s`, cpuArch, target)
	assertServerMaintenanceSettled(ctx, t, "evacuate", target, 2, "in maintenance, restore pending")
	assertInstanceNotOnMember(ctx, t, "evacuate", clusterName, instanceName, target)

	t.Logf("Restore %s", target)
	mustRun(t, `../bin/operations-center.linux.%s provisioning server system restore %s`, cpuArch, target)
	assertServerMaintenanceSettled(ctx, t, "restore", target, 0, "up to date")
	assertInstanceLocation(ctx, t, "restore", clusterName, instanceName, target)

	resp := mustRun(t, `../bin/operations-center.linux.%s warning list -f json | jq -r '[ .[] | select(.type == "Server lifecycle operation failed" and .scope.entity == "%s") ] | length'`, cpuArch, target)
	require.Equal(t, "0", resp.OutputTrimmed(), "expect no failed lifecycle operation warning for %s", target)
}

// assertServerMaintenanceSettled waits until the server is ready, has the
// given maintenance state and has an empty status detail. It then checks the
// update state. A status detail that is never cleared blocks later cluster
// operations.
func assertServerMaintenanceSettled(ctx context.Context, t *testing.T, phase string, name string, wantInMaintenance int, wantUpdateState string) {
	t.Helper()

	ok, err := waitForSuccessWithTimeout(ctx, t, fmt.Sprintf("%s: %s settled", phase, name), `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "%s") | (.version_data.in_maintenance // 0) == %d and .server_status == "ready" and .server_status_detail == ""'`, 5*time.Minute, cpuArch, name, wantInMaintenance)
	require.NoError(t, err, "%s: expect %s to settle", phase, name)

	if !ok {
		logWorkloadState(t, phase, incusOSClusterName)
		t.Fatalf("%s: %s did not reach in_maintenance %d with an empty status detail", phase, name, wantInMaintenance)
	}

	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning server show %s | grep '^Update Status:'`, cpuArch, name)
	require.Equal(t, "Update Status: "+wantUpdateState, resp.OutputTrimmed(), "%s: update state of %s", phase, name)
}

// serverRestoreCleanup restores the server if a failed step leaves it in
// maintenance. Later scenarios run on the same cluster.
func serverRestoreCleanup(t *testing.T, name string) func() {
	t.Helper()

	return func() {
		if noCleanup || (noCleanupOnError && t.Failed()) {
			return
		}

		// In t.Cleanup, t.Context() is cancelled, so we need a detached context.
		ctx, cancel := context.WithTimeout(context.Background(), strechedTimeout(6*time.Minute))
		defer cancel()

		stop := timeTrack(t, "server restore cleanup")
		defer stop()

		resp := runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "%s") | (.version_data.in_maintenance // 0) == 0'`, cpuArch, name)
		if resp.Success() {
			return
		}

		t.Logf("Restore %s during cleanup", name)
		resp = runWithContext(ctx, t, `../bin/operations-center.linux.%s provisioning server system restore --force %s`, cpuArch, name)
		if !resp.Success() {
			t.Logf("Failed to restore server %q during cleanup: %s", name, resp.Error())
			return
		}

		ok, err := waitForSuccessWithTimeout(ctx, t, fmt.Sprintf("cleanup: %s restored", name), `../bin/operations-center.linux.%s provisioning server list -f json | jq -r -e '.[] | select(.name == "%s") | (.version_data.in_maintenance // 0) == 0 and .server_status_detail == ""'`, 5*time.Minute, cpuArch, name)
		if err != nil || !ok {
			t.Logf("Server %q did not leave maintenance during cleanup: %v", name, err)
		}
	}
}

func assertInstanceLocation(ctx context.Context, t *testing.T, phase string, clusterName string, instanceName string, wantLocation string) {
	t.Helper()

	assertInstanceRunning(ctx, t, phase, clusterName, instanceName, fmt.Sprintf(`.location == "%s"`, wantLocation))
}

func assertInstanceNotOnMember(ctx context.Context, t *testing.T, phase string, clusterName string, instanceName string, member string) {
	t.Helper()

	assertInstanceRunning(ctx, t, phase, clusterName, instanceName, fmt.Sprintf(`.location != "%s"`, member))
}

func assertInstanceRunning(ctx context.Context, t *testing.T, phase string, clusterName string, instanceName string, locationFilter string) {
	t.Helper()

	ok, err := waitForSuccessWithTimeout(ctx, t, fmt.Sprintf("%s: %s running", phase, instanceName), `incus list %s: -f json | jq -r -e '.[] | select(.name == "%s") | .status == "Running" and %s'`, 2*time.Minute, clusterName, instanceName, locationFilter)
	require.NoError(t, err, "%s: expect instance %s to be running", phase, instanceName)

	if !ok {
		logWorkloadState(t, phase, clusterName)
		t.Fatalf("%s: instance %s is not running with %s", phase, instanceName, locationFilter)
	}
}
