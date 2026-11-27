package cluster

import (
	"context"
	"errors"
	"maps"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	"github.com/stretchr/testify/require"

	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/internal/warning"
	warningMock "github.com/FuturFusion/operations-center/internal/warning/mock"
	"github.com/FuturFusion/operations-center/shared/api"
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

// recordingTask is a leader task reporting its starts and stops.
type recordingTask struct {
	LeaderTask

	started chan context.Context
	stopped chan struct{}
}

func newRecordingTask(name string) recordingTask {
	r := recordingTask{started: make(chan context.Context, 10), stopped: make(chan struct{}, 10)}
	r.LeaderTask = LeaderTask{Name: name, Start: func(ctx context.Context) func(context.Context) error {
		r.started <- ctx

		return func(context.Context) error {
			r.stopped <- struct{}{}
			return nil
		}
	}}

	return r
}

// busyTasks is a stand-in for the other members, which run or stop the tasks it holds.
type busyTasks struct {
	mu    sync.Mutex
	tasks map[string][]string
}

// clear lets the other members stop all tasks.
func (b *busyTasks) clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tasks = nil
}

func (b *busyTasks) get(context.Context) (map[string][]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return maps.Clone(b.tasks), nil
}

func newLeaderTestMembership(ctx context.Context, cancel context.CancelFunc, g *leaderGateway, busy *busyTasks, tasks ...recordingTask) *membership {
	m := &membership{
		gateway:          g,
		ctx:              ctx,
		cancel:           cancel,
		busyMemberTasks:  busy.get,
		handoverTimeout:  leaderHandoverTimeout,
		handoverInterval: leaderHandoverInterval,
	}

	for _, t := range tasks {
		m.backgroundTasks = append(m.backgroundTasks, t.LeaderTask)
	}

	return m
}

// taskState returns the state of the task name, as this member reports it to the other members.
func taskState(m *membership, name string) string {
	for _, t := range m.taskStatus.list([]string{name}) {
		if t.Name == name {
			return t.State
		}
	}

	return ""
}

func TestMembership_checkLeadership(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	leaderTask := newRecordingTask("task")
	g := &leaderGateway{}
	m := newLeaderTestMembership(ctx, cancel, g, &busyTasks{}, leaderTask)

	// A member, which is not the leader, has no term and runs no leader tasks.
	m.checkLeadership(ctx)
	_, _, ok := m.LeaderContext("task")
	require.False(t, ok)
	require.Empty(t, leaderTask.started)

	// The leader tasks run on the term, which starts with the leadership.
	g.isLeader.Store(true)
	m.checkLeadership(ctx)

	term := <-leaderTask.started
	leaderCtx, done, ok := m.LeaderContext("task")
	require.True(t, ok)
	require.Equal(t, term, leaderCtx)
	require.NoError(t, term.Err())
	require.Equal(t, membershipapi.TaskRunning, taskState(m, "task"))

	// A further check during the term keeps the tasks running.
	m.checkLeadership(ctx)
	require.Empty(t, leaderTask.started)

	// The term ends with the leadership, which stops the tasks; the work still in flight keeps the task stopping.
	g.isLeader.Store(false)
	m.checkLeadership(ctx)

	require.Error(t, term.Err())
	<-leaderTask.stopped
	_, _, ok = m.LeaderContext("task")
	require.False(t, ok)
	require.Equal(t, membershipapi.TaskStopping, taskState(m, "task"))

	done()
	require.Equal(t, membershipapi.TaskStopped, taskState(m, "task"))

	// A new leadership starts a new term.
	g.isLeader.Store(true)
	m.checkLeadership(ctx)

	secondTerm := <-leaderTask.started
	require.NoError(t, secondTerm.Err())

	// A failed check leaves the leadership unknown, so the term goes on: every run of a task checks the leadership itself.
	g.err.Store(true)
	m.checkLeadership(ctx)

	require.NoError(t, secondTerm.Err())
	require.Empty(t, leaderTask.stopped)
	require.Empty(t, leaderTask.started)

	g.err.Store(false)
	m.checkLeadership(ctx)
	require.NoError(t, secondTerm.Err())
	require.Empty(t, leaderTask.started)

	// A term, which ended without the leadership moving, is started again by the next check.
	m.termMu.Lock()
	m.endTerm()
	m.termMu.Unlock()

	m.checkLeadership(ctx)
	thirdTerm := <-leaderTask.started
	require.NoError(t, thirdTerm.Err())

	// Once stopped, the leader tasks don't start again.
	require.Empty(t, m.stopLeaderTasks(ctx))
	require.Error(t, thirdTerm.Err())
	<-leaderTask.stopped

	m.checkLeadership(ctx)
	_, _, ok = m.LeaderContext("task")
	require.False(t, ok)
	require.Empty(t, leaderTask.started)
}

func TestMembership_handOverTasks_perTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		waiting := newRecordingTask("waiting")
		free := newRecordingTask("free")
		busy := &busyTasks{tasks: map[string][]string{"waiting": {"192.0.2.2:8443"}}}

		g := &leaderGateway{}
		g.isLeader.Store(true)
		m := newLeaderTestMembership(ctx, cancel, g, busy, waiting, free)

		// A task no other member runs starts right away, the other one waits.
		m.checkLeadership(ctx)
		synctest.Wait()

		require.Len(t, free.started, 1)
		require.Empty(t, waiting.started)

		// A task waiting to be started counts as stopped, so it never holds up another member.
		require.Equal(t, membershipapi.TaskStopped, taskState(m, "waiting"))

		time.Sleep(5 * time.Second)
		synctest.Wait()
		require.Empty(t, waiting.started, "the task waits, while another member runs it")

		// Once the other member stopped it, the task starts within a round.
		busy.clear()
		time.Sleep(m.handoverInterval)
		synctest.Wait()
		require.Len(t, waiting.started, 1)
		require.Equal(t, membershipapi.TaskRunning, taskState(m, "waiting"))

		require.Empty(t, m.stopLeaderTasks(ctx))
	})
}

func TestMembership_handOverTasks_leadershipLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		waiting := newRecordingTask("waiting")
		free := newRecordingTask("free")
		busy := &busyTasks{tasks: map[string][]string{"waiting": {"192.0.2.2:8443"}}}

		g := &leaderGateway{}
		g.isLeader.Store(true)
		m := newLeaderTestMembership(ctx, cancel, g, busy, waiting, free)

		m.checkLeadership(ctx)
		synctest.Wait()
		require.Len(t, free.started, 1)

		// The leadership moves on while waiting: the handover notices it in its next round, before the leadership check does.
		g.isLeader.Store(false)
		time.Sleep(m.handoverInterval)
		synctest.Wait()

		require.Len(t, free.stopped, 1, "the started task is stopped")
		require.Empty(t, waiting.started, "the waiting task is not started anymore")
		_, _, ok := m.LeaderContext("free")
		require.False(t, ok)

		busy.clear()
		time.Sleep(time.Minute)
		synctest.Wait()
		require.Empty(t, waiting.started, "the handover of an ended term starts nothing")

		// A new leadership starts a new handover.
		g.isLeader.Store(true)
		m.checkLeadership(ctx)
		synctest.Wait()
		require.Len(t, waiting.started, 1)
		require.Len(t, free.started, 2)

		require.Empty(t, m.stopLeaderTasks(ctx))
	})
}

func TestMembership_handOverTasks_checkFailed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		waiting := newRecordingTask("waiting")
		free := newRecordingTask("free")
		busy := &busyTasks{tasks: map[string][]string{"waiting": {"192.0.2.2:8443"}}}

		g := &leaderGateway{}
		g.isLeader.Store(true)
		m := newLeaderTestMembership(ctx, cancel, g, busy, waiting, free)

		m.checkLeadership(ctx)
		synctest.Wait()
		require.Len(t, free.started, 1)

		// A failed check during the handover neither ends the term nor starts a task, the handover goes on.
		g.err.Store(true)
		busy.clear()
		time.Sleep(5 * m.handoverInterval)
		synctest.Wait()

		require.Empty(t, free.stopped)
		require.Empty(t, waiting.started)

		g.err.Store(false)
		time.Sleep(m.handoverInterval)
		synctest.Wait()
		require.Len(t, waiting.started, 1)

		// The periodic check finds the term running and leaves it alone.
		m.checkLeadership(ctx)
		synctest.Wait()
		require.Len(t, free.started, 1)
		require.Len(t, waiting.started, 1)

		require.Empty(t, m.stopLeaderTasks(ctx))
	})
}

func TestMembership_handOverTasks_timeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		waiting := newRecordingTask("waiting")
		busy := &busyTasks{tasks: map[string][]string{"waiting": {"192.0.2.2:8443"}}}

		var emitted []warning.Warning
		var removed []api.WarningScope

		g := &leaderGateway{}
		g.isLeader.Store(true)
		m := newLeaderTestMembership(ctx, cancel, g, busy, waiting)
		m.warnings = &warningMock.WarningServiceMock{
			EmitFunc: func(ctx context.Context, w warning.Warning) {
				emitted = append(emitted, w)
			},
			RemoveStaleFunc: func(ctx context.Context, scope api.WarningScope, newWarnings warning.Warnings) {
				removed = append(removed, scope)
			},
		}

		m.checkLeadership(ctx)

		time.Sleep(m.handoverTimeout - time.Second)
		synctest.Wait()
		require.Empty(t, waiting.started)

		// Once the handover timed out, the task starts anyway and the member still running it is named in a warning.
		time.Sleep(2 * time.Second)
		synctest.Wait()
		require.Len(t, waiting.started, 1)
		require.Len(t, emitted, 1)
		require.Equal(t, api.WarningTypeLeaderHandoverIncomplete, emitted[0].Type)
		scope := api.WarningScope{Scope: emitted[0].Scope, EntityType: emitted[0].EntityType, Entity: emitted[0].Entity}
		require.Equal(t, leaderHandoverScope, scope)
		require.Contains(t, emitted[0].Messages[0], `"192.0.2.2:8443"`)
		require.Empty(t, removed)

		// A clean handover of the next term removes the warning.
		g.isLeader.Store(false)
		m.checkLeadership(ctx)
		<-waiting.stopped

		g.isLeader.Store(true)
		busy.clear()
		m.checkLeadership(ctx)
		synctest.Wait()
		require.Len(t, waiting.started, 2)
		require.Equal(t, []api.WarningScope{leaderHandoverScope}, removed)

		require.Empty(t, m.stopLeaderTasks(ctx))
	})
}

func TestMembership_handOverTasks_beforeStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		refresh := newRecordingTask("refresh")
		busy := &busyTasks{tasks: map[string][]string{"refresh": {"192.0.2.2:8443"}}}

		var events []string
		refresh.BeforeStart = func(context.Context) {
			events = append(events, "before start")
		}

		g := &leaderGateway{}
		g.isLeader.Store(true)
		m := newLeaderTestMembership(ctx, cancel, g, busy, refresh)

		// The hook waits like its task, while another member still runs it.
		m.checkLeadership(ctx)
		synctest.Wait()
		require.Empty(t, events)

		busy.clear()
		time.Sleep(m.handoverInterval)
		synctest.Wait()
		require.Equal(t, []string{"before start"}, events)
		require.Len(t, refresh.started, 1, "the task starts after its hook")

		// Every term runs the hook again.
		g.isLeader.Store(false)
		m.checkLeadership(ctx)
		<-refresh.stopped

		g.isLeader.Store(true)
		m.checkLeadership(ctx)
		synctest.Wait()
		require.Equal(t, []string{"before start", "before start"}, events)
		require.Len(t, refresh.started, 2)

		require.Empty(t, m.stopLeaderTasks(ctx))
	})
}
