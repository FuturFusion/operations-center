package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// ocClusterStoppedMemberCatchesUp changes the shared updates configuration
// while the daemon of a member is stopped. The member applies the stored
// configuration when it starts.
//
// The test changes the filter expression to an equivalent expression. So the
// change does not change which updates Operations Center keeps.
func ocClusterStoppedMemberCatchesUp(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())

	// Stop a member, which is not the leader. This keeps the leadership.
	leader := mustWaitOperationsCenterLeader(ctx, t, members[0], "")
	others := otherOperationsCenterMembers(members, leader)
	stopped, writer := others[0], others[1]

	original := mustOperationsCenterMemberSystemConfig(t, writer, "updates")
	originalFilter, ok := original["filter_expression"].(string)
	require.True(t, ok, "expect a filter expression in the updates configuration")

	changed := mustOperationsCenterMemberSystemConfig(t, writer, "updates")
	changed["filter_expression"] = "(" + originalFilter + ") && true"
	changedFilter := changed["filter_expression"]

	// The cleanups run in the reverse order. The configuration is restored
	// after the daemon runs again.
	t.Cleanup(systemConfigRestoreCleanup(t, tmpDir, writer, "updates", original))
	t.Cleanup(operationsCenterDaemonStartCleanup(t, stopped.name))

	mustStopOperationsCenterDaemon(t, stopped.name)
	mustWaitOperationsCenterMemberOffline(ctx, t, writer, stopped.name)

	t.Logf("Change the updates configuration through %s while %s is stopped", writer.name, stopped.name)

	filename := mustWriteSystemConfig(t, tmpDir, writer, "updates", changed)
	mustRun(t, `%s system updates edit < %s`, writer.oc, filename)

	for _, member := range otherOperationsCenterMembers(members, stopped.name) {
		require.Equal(t, changedFilter, mustOperationsCenterMemberSystemConfig(t, member, "updates")["filter_expression"], "expect %s to use the changed filter expression", member.name)
	}

	mustStartOperationsCenterDaemon(t, stopped.name)

	assertOperationsCenterClusterHealthy(ctx, t, members)

	t.Logf("Assert %s catches up", stopped.name)

	require.Equal(t, changedFilter, mustOperationsCenterMemberSystemConfig(t, stopped, "updates")["filter_expression"], "expect %s to apply the stored filter expression after its start", stopped.name)
}
