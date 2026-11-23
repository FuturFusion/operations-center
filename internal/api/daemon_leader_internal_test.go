package api

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	"github.com/stretchr/testify/require"

	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
)

// leaderGateway is a gateway only answering whether this member is the leader.
type leaderGateway struct {
	cowsqlcluster.Gateway

	isLeader atomic.Bool
	err      atomic.Bool
}

func (g *leaderGateway) IsLeader(context.Context) (bool, error) {
	if g.err.Load() {
		return false, errors.New("Leadership check failed")
	}

	return g.isLeader.Load(), nil
}

// leaderMembership is a membership service only answering the leadership and its term, counting the leader work by task.
type leaderMembership struct {
	clusteringSvc.MembershipService

	gateway *leaderGateway
	term    atomic.Pointer[context.Context]

	workMu sync.Mutex
	work   map[string]int
}

func (m *leaderMembership) Gateway() cowsqlcluster.Gateway {
	return m.gateway
}

func (m *leaderMembership) LeaderContext(name string) (context.Context, func(), bool) {
	term := m.term.Load()
	if term == nil || (*term).Err() != nil {
		return nil, nil, false
	}

	m.workMu.Lock()
	defer m.workMu.Unlock()

	if m.work == nil {
		m.work = map[string]int{}
	}

	m.work[name]++

	return *term, func() {
		m.workMu.Lock()
		defer m.workMu.Unlock()

		m.work[name]--
	}, true
}

// inFlight returns the leader work of the task name in flight.
func (m *leaderMembership) inFlight(name string) int {
	m.workMu.Lock()
	defer m.workMu.Unlock()

	return m.work[name]
}

type leaderTestKey struct{}

func TestDaemon_leaderOnly(t *testing.T) {
	t.Parallel()

	membership := &leaderMembership{gateway: &leaderGateway{}}
	d := &Daemon{membershipSvc: membership}

	term, endTerm := context.WithCancel(context.Background())
	defer endTerm()

	membership.term.Store(&term)

	runs := 0
	run := d.leaderOnly("deployment", func(context.Context) {
		require.Equal(t, 1, membership.inFlight("deployment"), "a run counts as work of its task")
		runs++
	})

	// A run is skipped unless this member is the leader at its start.
	run(t.Context())
	require.Zero(t, runs)

	membership.gateway.isLeader.Store(true)
	run(t.Context())
	require.Equal(t, 1, runs)
	require.Zero(t, membership.inFlight("deployment"), "a finished run is no work of its task anymore")

	// A run is skipped, while the task is not running on this member.
	membership.term.Store(nil)
	run(t.Context())
	require.Equal(t, 1, runs)
	membership.term.Store(&term)

	membership.gateway.err.Store(true)
	run(t.Context())
	require.Equal(t, 1, runs, "a failed leadership check skips the run")
}

func TestDaemon_leaderContext(t *testing.T) {
	t.Parallel()

	membership := &leaderMembership{gateway: &leaderGateway{}}
	d := &Daemon{membershipSvc: membership}

	term, endTerm := context.WithCancel(context.Background())
	defer endTerm()

	membership.term.Store(&term)

	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), leaderTestKey{}, "value"))
	defer cancel()

	// A member, which is not the leader, gets no context for leader work.
	_, _, ok := d.leaderContext(ctx, "task", slog.Default())
	require.False(t, ok)

	// A leader, whose term has not started yet, neither.
	membership.gateway.isLeader.Store(true)
	membership.term.Store(nil)
	_, _, ok = d.leaderContext(ctx, "task", slog.Default())
	require.False(t, ok)

	membership.term.Store(&term)
	leaderCtx, done, ok := d.leaderContext(ctx, "task", slog.Default())
	require.True(t, ok)

	// The leader work keeps the values of ctx, but outlives it.
	require.Equal(t, "value", leaderCtx.Value(leaderTestKey{}))
	cancel()
	require.NoError(t, leaderCtx.Err())

	// The leader work ends with the term.
	endTerm()

	select {
	case <-leaderCtx.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("The leader work does not end with the term")
	}

	_, _, ok = d.leaderContext(context.Background(), "task", slog.Default())
	require.False(t, ok, "an ended term gives no context for leader work")

	// Work of an ended term counts, until it is done.
	require.Equal(t, 1, membership.inFlight("task"))
	done()
	require.Zero(t, membership.inFlight("task"))

	// The work ends when it is done, even during the term.
	secondTerm, endSecondTerm := context.WithCancel(context.Background())
	defer endSecondTerm()

	membership.term.Store(&secondTerm)
	workCtx, workDone, ok := d.leaderContext(context.Background(), "task", slog.Default())
	require.True(t, ok)

	require.Equal(t, 1, membership.inFlight("task"), "the work counts as work of its task")
	workDone()
	require.Error(t, workCtx.Err())
	require.NoError(t, secondTerm.Err())
	require.Zero(t, membership.inFlight("task"), "done work is no work of its task anymore")
}
