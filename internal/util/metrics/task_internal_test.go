package metrics

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/taskmetrics"
)

type retryableTestErr struct{}

func (retryableTestErr) Error() string   { return "retryable" }
func (retryableTestErr) Retryable() bool { return true }

func gatherTask(t *testing.T, name string) taskmetrics.Series {
	t.Helper()

	reg := prometheus.NewRegistry()
	reg.MustRegister(taskCollector{})

	return taskmetrics.Gather(t, reg, name)
}

func TestTask_Observe(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name             string
		task             string
		enabledDuringRun []string
		ctx              context.Context
		err              error

		want               taskmetrics.Counts
		wantLastSuccessNew bool
	}{
		{
			name:             "success",
			task:             "test_observe_success",
			enabledDuringRun: []string{"test.task"},
			ctx:              context.Background(),
			err:              nil,

			want:               taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 0, DurationCount: 1},
			wantLastSuccessNew: true,
		},
		{
			name:             "error",
			task:             "test_observe_error",
			enabledDuringRun: []string{"test.task"},
			ctx:              context.Background(),
			err:              errors.New("boom"),

			want:               taskmetrics.Counts{ErrorsHard: 1, ErrorsRetryable: 0, DurationCount: 1},
			wantLastSuccessNew: false,
		},
		{
			name:             "retryable error",
			task:             "test_observe_retryable",
			enabledDuringRun: []string{"test.task"},
			ctx:              context.Background(),
			err:              fmt.Errorf("wrapped: %w", retryableTestErr{}),

			want:               taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 1, DurationCount: 1},
			wantLastSuccessNew: false,
		},
		{
			name:             "canceled context",
			task:             "test_observe_canceled",
			enabledDuringRun: []string{"test.task"},
			ctx:              canceledCtx,
			err:              nil,

			want:               taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 0, DurationCount: 0},
			wantLastSuccessNew: false,
		},
		{
			name:             "disabled component with error",
			task:             "test_observe_disabled_error",
			enabledDuringRun: nil,
			ctx:              context.Background(),
			err:              errors.New("boom"),

			want:               taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 0, DurationCount: 0},
			wantLastSuccessNew: false,
		},
		{
			name:             "disabled component with success",
			task:             "test_observe_disabled_success",
			enabledDuringRun: nil,
			ctx:              context.Background(),
			err:              nil,

			want:               taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 0, DurationCount: 0},
			wantLastSuccessNew: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			task := NewTask(RegisterComponent("test.task."+tc.task), tc.task)
			t.Cleanup(func() { SetEnabledComponents(nil) })

			task.Started(time.Minute)
			t.Cleanup(task.Stopped)

			// Make the start time distinguishable from a later success.
			task.lastSuccess.Set(1)

			SetEnabledComponents([]string{"test.task"})
			before := gatherTask(t, tc.task)

			SetEnabledComponents(tc.enabledDuringRun)

			// Run test
			task.Observe(tc.ctx, time.Now(), tc.err)

			// Assert
			SetEnabledComponents([]string{"test.task"})
			after := gatherTask(t, tc.task)
			require.True(t, after.Exposed, "an enabled task exposes its series")
			require.Equal(t, tc.want, after.Sub(before.Counts), "the run changes the counters")
			require.NotNil(t, after.LastSuccess, "a running task exposes the last success")
			require.Equal(t, tc.wantLastSuccessNew, *after.LastSuccess > 1, "only a success updates the last success, also while disabled")
		})
	}
}

func TestTask_Started(t *testing.T) {
	// Setup
	task := NewTask(RegisterComponent("test.task.test_started"), "test_started")
	t.Cleanup(func() { SetEnabledComponents(nil) })
	t.Cleanup(task.Stopped)

	SetEnabledComponents([]string{"test.task.test_started"})
	before := time.Now()

	// Run test
	task.Started(time.Minute)
	series := gatherTask(t, "test_started")

	// Assert
	require.NotNil(t, series.LastSuccess, "a running task exposes the last success")
	require.GreaterOrEqual(t, *series.LastSuccess, float64(before.Unix()), "a started task sets the last success to now, so an alert detects a task that never succeeds")
}

func TestTaskCollector(t *testing.T) {
	tests := []struct {
		name    string
		task    string
		enabled []string
		prepare func(task *Task)

		wantExposed     bool
		wantLastSuccess bool
		wantInterval    *float64
	}{
		{
			name:    "enabled and running",
			task:    "test_collector_running",
			enabled: []string{"test.task.test_collector_running"},
			prepare: func(*Task) {},

			wantExposed:     true,
			wantLastSuccess: true,
			wantInterval:    new(60.0),
		},
		{
			name:    "enabled by parent",
			task:    "test_collector_parent",
			enabled: []string{"test"},
			prepare: func(*Task) {},

			wantExposed:     true,
			wantLastSuccess: true,
			wantInterval:    new(60.0),
		},
		{
			name:    "enabled and stopped",
			task:    "test_collector_stopped",
			enabled: []string{"test.task.test_collector_stopped"},
			prepare: func(task *Task) { task.Stopped() },

			wantExposed:     true,
			wantLastSuccess: false,
			wantInterval:    nil,
		},
		{
			name:    "stopped and started again",
			task:    "test_collector_restarted",
			enabled: []string{"test.task.test_collector_restarted"},
			prepare: func(task *Task) {
				task.Stopped()
				task.Started(2 * time.Minute)
			},

			wantExposed:     true,
			wantLastSuccess: true,
			wantInterval:    new(120.0),
		},
		{
			name:    "other task enabled",
			task:    "test_collector_other",
			enabled: []string{"test.task.test_collector_running"},
			prepare: func(*Task) {},

			wantExposed:     false,
			wantLastSuccess: false,
			wantInterval:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			task := NewTask(RegisterComponent("test.task."+tc.task), tc.task)
			t.Cleanup(func() { SetEnabledComponents(nil) })

			task.Started(time.Minute)
			t.Cleanup(task.Stopped)

			tc.prepare(task)

			// Run test
			SetEnabledComponents(tc.enabled)
			series := gatherTask(t, tc.task)

			// Assert
			require.Equal(t, tc.wantExposed, series.Exposed, "series of the task exposed")
			require.Equal(t, tc.wantLastSuccess, series.LastSuccess != nil, "last success exposed")
			require.Equal(t, tc.wantInterval, series.Interval, "interval")
		})
	}
}

func TestNewTask_sameName(t *testing.T) {
	t.Parallel()

	// Setup
	first := NewTask(RegisterComponent("test.task.test_new_task"), "test_new_task")

	// Run test
	again := NewTask(RegisterComponent("test.task.test_new_task"), "test_new_task")

	// Assert
	require.Same(t, first, again, "the same name returns the same task")
}

func TestNewTask_panics(t *testing.T) {
	t.Parallel()

	// Setup
	NewTask(RegisterComponent("test.task.test_new_task_conflict"), "test_new_task_conflict")

	tests := []struct {
		name      string
		component func() *Component
		task      string
	}{
		{
			name:      "same name with another component",
			component: func() *Component { return RegisterComponent("test.task.other") },
			task:      "test_new_task_conflict",
		},
		{
			name:      "nil component",
			component: func() *Component { return nil },
			task:      "test_new_task_nil",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test and assert
			require.Panics(t, func() { NewTask(tc.component(), tc.task) }, "NewTask rejects a programming error")
		})
	}
}
