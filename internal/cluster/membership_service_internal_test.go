package cluster

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/task"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

// leaderGateway is a gateway reporting the leadership and recording its stop.
type leaderGateway struct {
	cowsqlcluster.Gateway

	leader    atomic.Bool
	leaderErr error
	stopErr   error

	stops   atomic.Int32
	timeout time.Duration
}

func (g *leaderGateway) IsLeader(context.Context) (bool, error) {
	return g.leader.Load(), g.leaderErr
}

func (g *leaderGateway) Stop(timeout time.Duration) error {
	g.stops.Add(1)
	g.timeout = timeout

	return g.stopErr
}

// countingTask counts the starts and stops of a background task.
type countingTask struct {
	starts  atomic.Int32
	stops   atomic.Int32
	stopErr error
}

func (c *countingTask) taskFunc(context.Context) func(context.Context) error {
	c.starts.Add(1)

	return func(context.Context) error {
		c.stops.Add(1)

		return c.stopErr
	}
}

func newTestMembership(t *testing.T, gateway cowsqlcluster.Gateway, tasks ...task.TaskFunc) *membership {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	return &membership{
		gateway:         gateway,
		ctx:             ctx,
		cancel:          cancel,
		backgroundTasks: tasks,
	}
}

func TestMembership_checkLeadership(t *testing.T) {
	t.Parallel()

	gateway := &leaderGateway{}
	bgTask := &countingTask{}
	m := newTestMembership(t, gateway, bgTask.taskFunc)

	// The tasks start once this member becomes the leader.
	gateway.leader.Store(true)
	m.checkLeadership(t.Context())
	m.checkLeadership(t.Context())
	require.Equal(t, int32(1), bgTask.starts.Load())
	require.Equal(t, int32(0), bgTask.stops.Load())

	gateway.leader.Store(false)
	m.checkLeadership(t.Context())
	require.Equal(t, int32(1), bgTask.stops.Load())

	gateway.leader.Store(true)
	m.checkLeadership(t.Context())
	require.Equal(t, int32(2), bgTask.starts.Load())

	// Without knowing the leader, the tasks stop, so they don't run on two members.
	gateway.leaderErr = boom.Error
	m.checkLeadership(t.Context())
	require.Equal(t, int32(2), bgTask.stops.Load())

	// A stopped service starts no tasks.
	gateway.leaderErr = nil
	m.cancel()
	m.checkLeadership(t.Context())
	require.Equal(t, int32(2), bgTask.starts.Load())
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

			gateway := &leaderGateway{stopErr: tc.gatewayStopErr}
			gateway.leader.Store(true)
			bgTask := &countingTask{stopErr: tc.taskStopErr}
			m := newTestMembership(t, gateway, bgTask.taskFunc)
			m.checkLeadership(t.Context())

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

	gateway := &leaderGateway{}
	gateway.leader.Store(true)
	bgTask := &countingTask{}
	m := newTestMembership(t, gateway, bgTask.taskFunc)

	m.StartLeaderTasks()

	// The first check runs right away.
	require.Eventually(t, func() bool { return bgTask.starts.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, m.Stop(t.Context()))
	require.Equal(t, int32(1), bgTask.stops.Load())
}
