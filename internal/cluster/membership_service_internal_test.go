package cluster

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

// stopGateway is a leader gateway recording its stop.
type stopGateway struct {
	leaderGateway

	stopErr error

	stops   atomic.Int32
	timeout time.Duration
}

func (g *stopGateway) Stop(timeout time.Duration) error {
	g.stops.Add(1)
	g.timeout = timeout

	return g.stopErr
}

// countingTask counts the starts and stops of a leader task.
type countingTask struct {
	starts  atomic.Int32
	stops   atomic.Int32
	stopErr error
}

func (c *countingTask) leaderTask() LeaderTask {
	return LeaderTask{Name: "task", Start: func(context.Context) func(context.Context) error {
		c.starts.Add(1)

		return func(context.Context) error {
			c.stops.Add(1)

			return c.stopErr
		}
	}}
}

func newStopTestMembership(t *testing.T, gateway *stopGateway, bgTask *countingTask) *membership {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	return &membership{
		gateway:          gateway,
		ctx:              ctx,
		cancel:           cancel,
		busyMemberTasks:  (&busyTasks{}).get,
		handoverTimeout:  leaderHandoverTimeout,
		handoverInterval: leaderHandoverInterval,
		backgroundTasks:  []LeaderTask{bgTask.leaderTask()},
	}
}

func TestMembership_Stop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ctxTimeout     time.Duration
		taskStopErr    error
		gatewayStopErr error

		assertErr   require.ErrorAssertionFunc
		wantTimeout func(t *testing.T, timeout time.Duration)
	}{
		{
			name: "success",

			assertErr: require.NoError,
			wantTimeout: func(t *testing.T, timeout time.Duration) {
				t.Helper()

				require.Equal(t, 5*time.Second, timeout)
			},
		},
		{
			name:       "success - deadline",
			ctxTimeout: time.Minute,

			assertErr: require.NoError,
			wantTimeout: func(t *testing.T, timeout time.Duration) {
				t.Helper()

				require.Greater(t, timeout, 5*time.Second)
				require.LessOrEqual(t, timeout, time.Minute)
			},
		},
		{
			name:        "error - background task",
			taskStopErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - gateway",
			gatewayStopErr: boom.Error,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to stop cluster database gateway")
				boom.ErrorIs(tt, err, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gateway := &stopGateway{stopErr: tc.gatewayStopErr}
			gateway.isLeader.Store(true)
			bgTask := &countingTask{stopErr: tc.taskStopErr}
			m := newStopTestMembership(t, gateway, bgTask)

			// The task starts once the other members stopped it.
			m.checkLeadership(t.Context())
			require.Eventually(t, func() bool { return bgTask.starts.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

			ctx := t.Context()
			if tc.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.ctxTimeout)
				defer cancel()
			}

			err := m.Stop(ctx)
			tc.assertErr(t, err)
			require.Equal(t, int32(1), bgTask.stops.Load())
			require.Equal(t, int32(1), gateway.stops.Load())
			if tc.wantTimeout != nil {
				tc.wantTimeout(t, gateway.timeout)
			}

			// Stopping again returns the first result.
			require.Equal(t, err, m.Stop(ctx))
			require.Equal(t, int32(1), gateway.stops.Load())

			// The leadership check doesn't start the tasks again.
			m.checkLeadership(t.Context())
			require.Equal(t, int32(1), bgTask.starts.Load())
		})
	}
}

func TestMembership_StartLeaderTasks(t *testing.T) {
	t.Parallel()

	gateway := &stopGateway{}
	gateway.isLeader.Store(true)
	bgTask := &countingTask{}
	m := newStopTestMembership(t, gateway, bgTask)

	m.StartLeaderTasks()

	// The first check runs right away.
	require.Eventually(t, func() bool { return bgTask.starts.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, m.Stop(t.Context()))
	require.Equal(t, int32(1), bgTask.stops.Load())
}
