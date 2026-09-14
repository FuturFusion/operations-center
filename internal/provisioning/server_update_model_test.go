package provisioning_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestServerUpdate_claim(t *testing.T) {
	now := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)

	// A server, that does not take part in a run, has no record at all, and every
	// accessor tolerates that.
	var absent *provisioning.ServerUpdate
	require.False(t, absent.IsActive())
	require.False(t, absent.RebootOwed())
	require.False(t, absent.KeepsEvacuated())
	require.Equal(t, provisioning.ServerUpdateStepNone, absent.InFlightStep(now))

	serverUpdate := &provisioning.ServerUpdate{StartedAt: now}

	// A triggered step is in flight for as long as it has been granted.
	serverUpdate.Claim(now, provisioning.ServerUpdateStepEvacuate)
	require.Equal(t, 1, serverUpdate.Retries)
	require.Equal(t, provisioning.ServerUpdateStepEvacuate, serverUpdate.InFlightStep(now))
	require.Equal(t, provisioning.ServerUpdateStepEvacuate, serverUpdate.InFlightStep(now.Add(config.ClusterRollingUpdateEvacuateTimeout)))

	// Beyond it, nothing is going to report its outcome anymore.
	require.Equal(t, provisioning.ServerUpdateStepNone, serverUpdate.InFlightStep(now.Add(config.ClusterRollingUpdateEvacuateTimeout+time.Second)))

	// A reported failure ends the attempt but keeps the budget, so the retries of
	// one step accumulate no matter how far apart the attempts are.
	serverUpdate.Fail(provisioning.ServerUpdateStepEvacuate, boom.Error)
	require.Equal(t, 1, serverUpdate.Retries)
	require.Equal(t, boom.Error.Error(), serverUpdate.LastError)
	require.Equal(t, provisioning.ServerUpdateStepNone, serverUpdate.InFlightStep(now))

	serverUpdate.Claim(now.Add(time.Hour), provisioning.ServerUpdateStepEvacuate)
	require.Equal(t, 2, serverUpdate.Retries)

	// An outcome reported for another step does not retire the current one.
	serverUpdate.Release(provisioning.ServerUpdateStepRestore)
	require.Equal(t, provisioning.ServerUpdateStepEvacuate, serverUpdate.Step)

	// Advancing to the next step starts with a full budget again.
	serverUpdate.Release(provisioning.ServerUpdateStepEvacuate)
	require.Equal(t, provisioning.ServerUpdateStepNone, serverUpdate.Step)
	require.Zero(t, serverUpdate.Retries)
	require.Empty(t, serverUpdate.LastError)

	serverUpdate.Claim(now, provisioning.ServerUpdateStepRestore)
	require.Equal(t, 1, serverUpdate.Retries)
}
