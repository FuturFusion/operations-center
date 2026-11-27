package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	leaderHandoverChannelName  = "e2e-leader-handover"
	leaderHandoverInstanceName = "e2e-leader-handover"
)

// ocClusterLeaderHandover stops the daemon of the leader. Another member takes
// over and handles the changes. The former leader catches up after its start.
//
// The test stops the daemon instead of rebooting the VM. A reboot replaces the
// local build with the daemon shipped with IncusOS.
func ocClusterLeaderHandover(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())

	formerLeader := operationsCenterMemberByName(t, members, mustWaitOperationsCenterLeader(ctx, t, members[0], ""))
	others := otherOperationsCenterMembers(members, formerLeader.name)

	// The cleanups run in the reverse order. The channel is removed after the
	// daemon runs again.
	t.Cleanup(leftoverCleanup(t, "channel "+leaderHandoverChannelName, `../bin/operations-center.linux.%s provisioning channel remove %s`, cpuArch, leaderHandoverChannelName))
	t.Cleanup(operationsCenterDaemonStartCleanup(t, formerLeader.name))

	mustStopOperationsCenterDaemon(t, formerLeader.name)

	newLeader := mustWaitOperationsCenterLeader(ctx, t, others[0], formerLeader.name)
	require.Equal(t, newLeader, mustWaitOperationsCenterLeader(ctx, t, others[1], formerLeader.name), "expect the remaining members to agree on the new leader")

	t.Log("Change data while the former leader is stopped")

	mustRun(t, `%s provisioning channel add %s --description "added without the former leader"`, others[0].oc, leaderHandoverChannelName)
	mustRun(t, `%s provisioning channel show %s -f json | jq -e '.description == "added without the former leader"'`, others[1].oc, leaderHandoverChannelName)

	mustStartOperationsCenterDaemon(t, formerLeader.name)

	assertOperationsCenterClusterHealthy(ctx, t, members)

	t.Log("Assert the former leader catches up")

	mustRun(t, `%s provisioning channel show %s -f json | jq -e '.description == "added without the former leader"'`, formerLeader.oc, leaderHandoverChannelName)

	// The former leader forwards the change to the new leader.
	mustRun(t, `%s provisioning channel remove %s`, formerLeader.oc, leaderHandoverChannelName)

	for _, member := range members {
		mustRun(t, `%s provisioning channel list -f json | jq -e '[ .[] | select(.name == "%s") ] | length == 0'`, member.oc, leaderHandoverChannelName)
	}
}

// ocLeaderHandoverWithManagedCluster stops the daemon of the leader, while
// Operations Center manages the cluster incus-os-cluster. The new leader
// runs the background tasks. The test checks the monitor of the lifecycle
// events. It updates the inventory, when an instance is added or removed.
func ocLeaderHandoverWithManagedCluster(ctx context.Context, t *testing.T, tmpDir string) {
	t.Helper()

	stop := timeTrack(t)
	defer stop()

	members := mustTrustOperationsCenterMembers(ctx, t, tmpDir, operationsCenterVMNames())

	formerLeader := operationsCenterMemberByName(t, members, mustWaitOperationsCenterLeader(ctx, t, members[0], ""))
	others := otherOperationsCenterMembers(members, formerLeader.name)

	// The cleanups run in the reverse order. The instance is removed after
	// the daemon runs again.
	t.Cleanup(workloadCleanup(t, incusOSClusterName, []string{leaderHandoverInstanceName}))
	t.Cleanup(operationsCenterDaemonStartCleanup(t, formerLeader.name))

	mustStopOperationsCenterDaemon(t, formerLeader.name)

	mustWaitOperationsCenterLeader(ctx, t, others[0], formerLeader.name)

	t.Log("Launch an instance while the former leader is stopped")

	mustRun(t, `incus launch images:alpine/edge %s:%s`, incusOSClusterName, leaderHandoverInstanceName)

	assertLeaderHandoverInstances(ctx, t, others[1], 1)

	mustStartOperationsCenterDaemon(t, formerLeader.name)

	assertOperationsCenterClusterHealthy(ctx, t, members)

	assertLeaderHandoverInstances(ctx, t, formerLeader, 1)

	t.Log("Delete the instance after the former leader is back")

	mustRun(t, `incus delete --force %s:%s`, incusOSClusterName, leaderHandoverInstanceName)

	for _, member := range members {
		assertLeaderHandoverInstances(ctx, t, member, 0)
	}
}

// assertLeaderHandoverInstances waits until member lists count instances
// called leaderHandoverInstanceName in the inventory of the cluster
// incus-os-cluster. Only the leader
// handles the lifecycle events, which update the inventory. After a
// leadership change, the new leader starts the monitor after at most 30
// seconds.
func assertLeaderHandoverInstances(ctx context.Context, t *testing.T, member operationsCenterMember, count int) {
	t.Helper()

	ok, err := waitForSuccessWithTimeout(ctx, t, "inventory instance list", `%s inventory instance list -f json | jq -e '[ .[] | select(.cluster == "%s" and .name == "%s") ] | length == %d'`, 2*time.Minute, member.oc, incusOSClusterName, leaderHandoverInstanceName, count)
	require.NoError(t, err, "Failed to list the inventory instances through %s", member.name)
	require.True(t, ok, "expect %s to list %d instances %q in the inventory of %s", member.name, count, leaderHandoverInstanceName, incusOSClusterName)
}
