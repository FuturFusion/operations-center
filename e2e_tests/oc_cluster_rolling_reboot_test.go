package e2e

import (
	"context"
	"testing"
)

// ocClusterRollingRebootThroughMembers starts the rolling reboot of the
// cluster incus-os-cluster through a member, which is not the leader. The
// member forwards the request to the leader. A third member reports the
// progress.
func ocClusterRollingRebootThroughMembers(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())

	leader := mustWaitOperationsCenterLeader(ctx, t, members[0], "")
	others := otherOperationsCenterMembers(members, leader)
	trigger, progress := others[0], others[1]

	t.Logf("Start the rolling reboot through %s and follow the progress through %s, the leader is %s", trigger.name, progress.name, leader)

	assertClusterRollingReboot(ctx, t, trigger.oc, progress.oc)
}
