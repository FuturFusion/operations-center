package e2e

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createClusterAndThenClusterReboot(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	clusterName := "incus-os-cluster"
	names := []string{"IncusOS01", "IncusOS02", "IncusOS03"}

	createCluster(names)(ctx, t, tmpDir)

	t.Log("Start some small VMs for the cluster to have some minimal workload.")
	instanceNames := make([]string, 0, len(names))
	for i := range names {
		instanceNames = append(instanceNames, fmt.Sprintf("mini-alpine-%d", i))
		mustRun(t, `incus launch --vm images:alpine/edge %s:%s -c limits.cpu=1 -c limits.memory=256MiB -c security.secureboot=false -c migration.stateful=true`, clusterName, instanceNames[i])
	}

	assertWorkloadRunning(ctx, t, "Reboot cluster - pre reboot workload", clusterName, instanceNames)

	t.Log("Reboot cluster - pre reboot state")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list`, cpuArch)
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster show %s`, cpuArch, clusterName)

	// Nothing may be pending, otherwise the reboot would not be on demand.
	resp := mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | (.update_status.needs_update // []) + (.update_status.needs_reboot // []) | join(",")'`, cpuArch, clusterName)
	require.Emptyf(t, resp.OutputTrimmed(), "Reboot cluster: servers already need an update or a reboot before the test starts: %s", resp.OutputTrimmed())

	stopReboot := timeTrack(t, "cluster reboot")
	defer stopReboot()

	t.Log("Update post_restore_delay")
	mustRun(t, `EDITOR='sed -i "s/    post_restore_delay:.*/    post_restore_delay: 20s/"' script -q -c '../bin/operations-center.linux.%s provisioning cluster edit %s' /dev/null`, cpuArch, clusterName)

	t.Log("Reboot cluster - trigger rolling reboot")
	mustRun(t, `../bin/operations-center.linux.%s provisioning cluster reboot %s`, cpuArch, clusterName)

	ctx, cancel := context.WithTimeout(ctx, strechedTimeout(30*time.Minute))
	defer cancel()

	previousStatusDescription := ""
	previousStep := 0
	previousTotalSteps := 0

	for {
		resp := mustRunQuiet(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | .update_status.in_progress_status.error'`, cpuArch, clusterName)
		if resp.OutputTrimmed() != "" {
			t.Fatalf("Reboot cluster failed: %s", resp.OutputTrimmed())
		}

		resp = mustRunQuiet(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | .update_status.in_progress_status.in_progress'`, cpuArch, clusterName)
		if resp.OutputTrimmed() == "" {
			break
		}

		resp = mustRunQuiet(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | .update_status.in_progress_status.status_description // ""'`, cpuArch, clusterName)
		statusDescription := resp.OutputTrimmed()

		if statusDescription != previousStatusDescription {
			t.Logf("Reboot cluster: %s", statusDescription)
		}

		previousStatusDescription = statusDescription

		// The progress reported to the user must never move backwards.
		match := clusterUpdateStateRegexp.FindStringSubmatch(statusDescription)
		if match != nil {
			step, err := strconv.Atoi(match[1])
			require.NoError(t, err)

			totalSteps, err := strconv.Atoi(match[2])
			require.NoError(t, err)

			require.GreaterOrEqualf(t, step, previousStep, "Reboot cluster: progress moved backwards from %d to %d of %d", previousStep, step, totalSteps)

			if previousTotalSteps != 0 {
				require.Equalf(t, previousTotalSteps, totalSteps, "Reboot cluster: total number of steps changed from %d to %d", previousTotalSteps, totalSteps)
			}

			previousStep = step
			previousTotalSteps = totalSteps
		}

		if debug {
			resp = mustRun(t, `../bin/operations-center.linux.%s provisioning server list -f json | jq '[ .[] | { "server_status": .server_status, "server_status_detail": .server_status_detail, "version_data": .version_data } ]'`, cpuArch)
			debugf("per server status: %s", resp.Output())
		}

		select {
		case <-ctx.Done():
			t.Fatalf("Reboot deadline reached: %v", ctx.Err())
			return

		case <-time.After(1 * time.Second):
		}
	}

	require.NotZero(t, previousTotalSteps, "Reboot cluster: no progress has been reported at all")
	require.Equal(t, previousTotalSteps, previousStep, "Reboot cluster: the last reported progress is not the last step")

	resp = mustRun(t, `../bin/operations-center.linux.%s provisioning cluster list -f json | jq -r '.[] | select(.name == "%s") | (.update_status.needs_update // []) + (.update_status.needs_reboot // []) + (.update_status.in_maintenance // []) | join(",")'`, cpuArch, clusterName)
	require.Emptyf(t, resp.OutputTrimmed(), "Reboot cluster: servers still need an update or a reboot or are in maintenance: %s", resp.OutputTrimmed())

	assertWorkloadRunning(ctx, t, "Reboot cluster - post reboot workload", clusterName, instanceNames)

	t.Log("Reboot cluster - rolling reboot completed")
}

const workloadAgentTimeout = 3 * time.Minute

// assertWorkloadRunning verifies, that all the given instances of the cluster are
// up and running and that their incus agent answers.
func assertWorkloadRunning(ctx context.Context, t *testing.T, phase string, clusterName string, instanceNames []string) { //nolint:unparam
	t.Helper()

	stop := timeTrack(t, phase)
	defer stop()

	ok, err := waitForSuccessWithTimeout(
		ctx, t, phase,
		`incus list %s: -f json | jq -r -e '[ .[] | select(.name as $n | %s | index($n)) | select(.status == "Running") ] | length == %d'`,
		2*time.Minute, clusterName, asJSON(t, instanceNames), len(instanceNames),
	)
	require.NoError(t, err, "%s: failed to get the state of the instances %v", phase, instanceNames)

	if !ok {
		logWorkloadState(t, phase, clusterName)

		t.Fatalf("%s: not all of the instances %v are running", phase, instanceNames)
	}

	for _, instanceName := range instanceNames {
		ok, err := waitForSuccessWithTimeout(
			ctx, t, fmt.Sprintf("%s: incus agent of %q", phase, instanceName),
			`incus exec %s:%s -- true`,
			workloadAgentTimeout, clusterName, instanceName,
		)
		require.NoError(t, err, "%s: failed to reach the incus agent of the instance %q", phase, instanceName)

		if !ok {
			logWorkloadState(t, phase, clusterName)

			t.Fatalf("%s: the incus agent of the instance %q does not answer", phase, instanceName)
		}
	}
}

// logWorkloadState collects, what is worth knowing about a workload, which did
// not reach the expected state.
func logWorkloadState(t *testing.T, phase string, clusterName string) {
	t.Helper()

	resp := run(t, `incus list %s: -f json | jq -c '[ .[] | { name: .name, status: .status, location: .location } ]'`, clusterName)
	t.Logf("%s: instances of cluster %q: %s", phase, clusterName, resp.OutputTrimmed())

	resp = run(t, `../bin/operations-center.linux.%s warning list`, cpuArch)
	t.Logf("%s: warnings:\n%s", phase, resp.Output())

	printServerList(t)
}
