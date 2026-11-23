package cluster

import (
	"context"
	"slices"
	"strings"
	"sync"

	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/internal/util/task"
)

// LeaderTask is a background task run on the cluster leader only.
type LeaderTask struct {
	// Name identifies the task to the other members.
	Name string

	// Start starts the task and returns the function stopping it.
	Start task.TaskFunc

	// BeforeStart runs right before the task starts on a new leader, once no other member runs it anymore, if set.
	BeforeStart func(ctx context.Context)
}

// leaderTaskStatus records the leader work of this member, so a new leader can wait for it to end.
type leaderTaskStatus struct {
	mu    sync.Mutex
	tasks map[string]*leaderTaskState
}

type leaderTaskState struct {
	started  bool
	inFlight int
}

// state returns the state of the task name. The caller must hold mu.
func (s *leaderTaskStatus) state(name string) *leaderTaskState {
	if s.tasks == nil {
		s.tasks = map[string]*leaderTaskState{}
	}

	state, ok := s.tasks[name]
	if !ok {
		state = &leaderTaskState{}
		s.tasks[name] = state
	}

	return state
}

// setStarted records whether the task name is started on this member.
func (s *leaderTaskStatus) setStarted(name string, started bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state(name).started = started
}

// begin records work of the task name in flight and returns the function ending it, false if the task is not started.
func (s *leaderTaskStatus) begin(name string) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.state(name)
	if !state.started {
		return nil, false
	}

	state.inFlight++

	return sync.OnceFunc(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		state.inFlight--
	}), true
}

// list returns the state of each task, including the tasks in names, which have not been started yet.
func (s *leaderTaskStatus) list(names []string) []membershipapi.InternalClusterTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range names {
		s.state(name)
	}

	tasks := make([]membershipapi.InternalClusterTask, 0, len(s.tasks))
	for name, state := range s.tasks {
		taskState := membershipapi.TaskStopped
		switch {
		case state.started:
			taskState = membershipapi.TaskRunning

		case state.inFlight > 0:
			taskState = membershipapi.TaskStopping
		}

		tasks = append(tasks, membershipapi.InternalClusterTask{Name: name, State: taskState})
	}

	slices.SortFunc(tasks, func(a, b membershipapi.InternalClusterTask) int {
		return strings.Compare(a.Name, b.Name)
	})

	return tasks
}
