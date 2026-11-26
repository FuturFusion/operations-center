package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/taskmetrics"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func TestComponentMiddleware(t *testing.T) {
	t.Parallel()

	logBuf := &log.SyncBuffer{}
	ctx := logger.ContextWithTestLogger(t, logBuf, true, false)

	handler := componentMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "handling")
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, "/1.0", nil))

	require.Contains(t, logBuf.String(), "component=api", "a request is attributed to the api component")
}

func TestComponentDaemonTask(t *testing.T) {
	t.Parallel()

	logBuf := &log.SyncBuffer{}
	ctx := logger.ContextWithTestLogger(t, logBuf, true, false)

	ran := make(chan struct{})
	start := componentTask(componentTaskRefreshInventory, time.Hour, time.Second, func(ctx context.Context) error {
		slog.InfoContext(ctx, "task body")
		close(ran)

		return nil
	})

	stop := start(ctx)
	<-ran
	require.NoError(t, stop(ctx), "the task stops within its deadline")

	require.Contains(t, logBuf.String(), "component=daemon.task.refresh_inventory", "a background task is attributed to its own component")
}

func gatherTask(t *testing.T, name string) taskmetrics.Series {
	t.Helper()

	return taskmetrics.Gather(t, metrics.Gatherer(), name)
}

func TestComponentTask_metrics(t *testing.T) {
	tests := []struct {
		name    string
		task    string
		enabled []string
		run     func(cancel context.CancelFunc) error

		want taskmetrics.Counts
	}{
		{
			name:    "success",
			task:    "test_success",
			enabled: []string{"daemon.task.test_success"},
			run:     func(context.CancelFunc) error { return nil },

			want: taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 0, DurationCount: 1},
		},
		{
			name:    "error",
			task:    "test_error",
			enabled: []string{"daemon.task.test_error"},
			run:     func(context.CancelFunc) error { return errors.New("boom") },

			want: taskmetrics.Counts{ErrorsHard: 1, ErrorsRetryable: 0, DurationCount: 1},
		},
		{
			name:    "retryable error",
			task:    "test_retryable_error",
			enabled: []string{"daemon.task.test_retryable_error"},
			run:     func(context.CancelFunc) error { return domain.NewRetryableErr(errors.New("boom")) },

			want: taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 1, DurationCount: 1},
		},
		{
			name:    "canceled context",
			task:    "test_canceled",
			enabled: []string{"daemon.task.test_canceled"},
			run: func(cancel context.CancelFunc) error {
				cancel()

				return context.Canceled
			},

			want: taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 0, DurationCount: 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			component := newTaskComponent(
				logger.RegisterComponent("daemon.task."+tc.task),
				metrics.RegisterComponent("daemon.task."+tc.task),
			)
			t.Cleanup(func() { metrics.SetEnabledComponents(nil) })

			metrics.SetEnabledComponents(tc.enabled)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			ran := make(chan struct{})
			start := componentTask(component, time.Hour, time.Second, func(context.Context) error {
				defer close(ran)

				return tc.run(cancel)
			})

			metrics.SetEnabledComponents([]string{"daemon.task." + tc.task})
			before := gatherTask(t, tc.task)
			metrics.SetEnabledComponents(tc.enabled)

			// Run test
			stop := start(ctx)
			<-ran
			// The run is recorded after f returns. Stop the task only then, since
			// stopping it cancels the context and discards the run.
			require.Eventually(t, func() bool {
				after := gatherTask(t, tc.task)
				return after.Sub(before.Counts) == tc.want
			}, time.Second, time.Millisecond, "the run is recorded")
			require.NoError(t, stop(t.Context()), "the task stops within its deadline")

			// Assert
			metrics.SetEnabledComponents([]string{"daemon.task." + tc.task})
			after := gatherTask(t, tc.task)
			require.Equal(t, tc.want, after.Sub(before.Counts), "the metrics of the task after it stopped")
		})
	}
}

func TestComponentTask_metricsDisabled(t *testing.T) {
	// Setup
	component := newTaskComponent(
		logger.RegisterComponent("daemon.task.test_disabled"),
		metrics.RegisterComponent("daemon.task.test_disabled"),
	)
	t.Cleanup(func() { metrics.SetEnabledComponents(nil) })

	metrics.SetEnabledComponents([]string{"daemon.task.test_disabled"})
	before := gatherTask(t, "test_disabled")
	metrics.SetEnabledComponents(nil)

	var calls atomic.Int32
	secondRun := make(chan struct{})
	start := componentTask(component, time.Millisecond, time.Second, func(ctx context.Context) error {
		call := calls.Add(1)
		if call == 1 {
			return errors.New("boom")
		}

		// The task runs f synchronously. The second run therefore starts only
		// after the first run is observed.
		if call == 2 {
			close(secondRun)
		}

		<-ctx.Done()

		return ctx.Err()
	})

	// Run test
	stop := start(t.Context())
	t.Cleanup(func() { _ = stop(context.Background()) })

	select {
	case <-secondRun:
	case <-time.After(5 * time.Second):
		t.Fatal("the second run did not start")
	}

	// Assert
	metrics.SetEnabledComponents([]string{"daemon.task.test_disabled"})
	after := gatherTask(t, "test_disabled")
	require.Equal(t, taskmetrics.Counts{ErrorsHard: 0, ErrorsRetryable: 0, DurationCount: 0}, after.Sub(before.Counts), "a run while the component is disabled records no error and no duration")
	require.NoError(t, stop(t.Context()), "the task stops within its deadline")
}

func TestComponentTask_running(t *testing.T) {
	// Setup
	component := newTaskComponent(
		logger.RegisterComponent("daemon.task.test_running"),
		metrics.RegisterComponent("daemon.task.test_running"),
	)
	t.Cleanup(func() { metrics.SetEnabledComponents(nil) })

	metrics.SetEnabledComponents([]string{"daemon.task.test_running"})

	start := componentTask(component, time.Hour, time.Second, func(context.Context) error { return nil })

	// Run test
	stop := start(t.Context())
	whileRunning := gatherTask(t, "test_running").Interval
	require.NoError(t, stop(t.Context()), "the task stops within its deadline")
	afterStop := gatherTask(t, "test_running").Interval

	// Assert
	require.Equal(t, new(3600.0), whileRunning, "a running task exposes its interval")
	require.Nil(t, afterStop, "a stopped task, for example after the loss of the cluster leadership, exposes no interval")
}

func TestNewTaskComponent_mismatch(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		newTaskComponent(
			logger.RegisterComponent("daemon.task.test_mismatch"),
			metrics.RegisterComponent("daemon.task.test_mismatch_other"),
		)
	}, "differing names of the log and the metrics component are a bug")
}

func TestValidateComponentLevels(t *testing.T) {
	require.NoError(t, validateComponentLevels(t.Context(), apisystem.SettingsLocal{
		SettingsLocalPut: apisystem.SettingsLocalPut{
			LogLevels: map[string]string{"daemon.task": "DEBUG"},
		},
	}), "a parent of a registered component is accepted")

	err := validateComponentLevels(t.Context(), apisystem.SettingsLocal{
		SettingsLocalPut: apisystem.SettingsLocalPut{
			LogLevels: map[string]string{"daemon.tsak": "DEBUG"},
		},
	})

	var validationErr domain.ErrValidation

	require.ErrorAs(t, err, &validationErr, "an unknown component is a validation error, so the API answers with 400")
	require.ErrorContains(t, err, `"daemon.tsak"`, "the rejection names the unknown component")
}

// requireValidationErrorContains asserts a validation error that names the
// given value. The API answers a validation error with 400.
func requireValidationErrorContains(contains string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, msgAndArgs ...any) {
		var validationErr domain.ErrValidation

		require.ErrorAs(t, err, &validationErr, msgAndArgs...)
		require.ErrorContains(t, err, contains, msgAndArgs...)
	}
}

func TestValidateMetricsComponents(t *testing.T) {
	tests := []struct {
		name       string
		components []string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:       "registered component",
			components: []string{"runtime"},

			assertErr: require.NoError,
		},
		{
			name:       "unknown component",
			components: []string{"runtmie"},

			assertErr: requireValidationErrorContains(`"runtmie"`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			settings := apisystem.SettingsLocal{
				SettingsLocalPut: apisystem.SettingsLocalPut{
					MetricsComponents: tc.components,
				},
			}

			// Run test
			err := validateMetricsComponents(t.Context(), settings)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestRegisterComponentsValidation(t *testing.T) {
	tests := []struct {
		name     string
		settings apisystem.SettingsLocalPut

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "known components",
			settings: apisystem.SettingsLocalPut{
				LogLevel:          "WARN",
				LogLevels:         map[string]string{"daemon.task": "DEBUG"},
				MetricsEnabled:    true,
				MetricsComponents: []string{"runtime"},
			},

			assertErr: require.NoError,
		},
		{
			name: "unknown log component",
			settings: apisystem.SettingsLocalPut{
				LogLevel:  "WARN",
				LogLevels: map[string]string{"daemon.tsak": "DEBUG"},
			},

			assertErr: requireValidationErrorContains(`"daemon.tsak"`),
		},
		{
			name: "unknown metrics component",
			settings: apisystem.SettingsLocalPut{
				LogLevel:          "WARN",
				MetricsEnabled:    true,
				MetricsComponents: []string{"runtmie"},
			},

			assertErr: requireValidationErrorContains(`"runtmie"`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t, &mock.EnvironmentMock{IsIncusOSFunc: func() bool { return false }}, nil)
			t.Cleanup(func() { metrics.Apply(false, nil) })

			registerComponentsValidation()

			// Run test
			err := config.UpdateLocalSettings(t.Context(), tc.settings)

			// Assert
			tc.assertErr(t, err, "the settings update runs the registered component checks")
		})
	}
}
