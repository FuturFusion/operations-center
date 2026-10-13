package api

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func TestComponentMiddleware(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", true, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", true, false, true), "the log levels must not leak into other tests")
	})

	handler := componentMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "handling")
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/1.0", nil))

	require.Contains(t, logBuf.String(), "component=api", "a request is attributed to the api component")
}

func TestComponentDaemonTask(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", true, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", true, false, true), "the log levels must not leak into other tests")
	})

	componentTask(componentTaskRefreshInventory, func(ctx context.Context) {
		slog.InfoContext(ctx, "task body")
	})(t.Context())

	require.Contains(t, logBuf.String(), "component=daemon.task.refresh_inventory", "a background task is attributed to its own component")
}

func TestValidateComponentLevels(t *testing.T) {
	require.NoError(t, validateComponentLevels(t.Context(), apisystem.Settings{
		SettingsPut: apisystem.SettingsPut{
			LogLevels: map[string]string{"daemon.task": "DEBUG"},
		},
	}), "a parent of a registered component is accepted")

	err := validateComponentLevels(t.Context(), apisystem.Settings{
		SettingsPut: apisystem.SettingsPut{
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
			settings := apisystem.Settings{
				SettingsPut: apisystem.SettingsPut{
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
		settings apisystem.SettingsPut

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "known components",
			settings: apisystem.SettingsPut{
				LogLevel:          "WARN",
				LogLevels:         map[string]string{"daemon.task": "DEBUG"},
				MetricsEnabled:    true,
				MetricsComponents: []string{"runtime"},
			},

			assertErr: require.NoError,
		},
		{
			name: "unknown log component",
			settings: apisystem.SettingsPut{
				LogLevel:  "WARN",
				LogLevels: map[string]string{"daemon.tsak": "DEBUG"},
			},

			assertErr: requireValidationErrorContains(`"daemon.tsak"`),
		},
		{
			name: "unknown metrics component",
			settings: apisystem.SettingsPut{
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
			err := config.UpdateSettings(t.Context(), tc.settings)

			// Assert
			tc.assertErr(t, err, "the settings update runs the registered component checks")
		})
	}
}
