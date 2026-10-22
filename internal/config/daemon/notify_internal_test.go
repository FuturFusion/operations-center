package config

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_notify_smoke(t *testing.T) {
	InitTest(t, &mock.EnvironmentMock{IsIncusOSFunc: func() bool { return false }}, nil)

	var (
		acmeEmits    int
		networkEmits []string
	)

	lifecycle.SecurityACMEUpdateSignal.AddListener(func(_ context.Context, _ system.SecurityACME) {
		acmeEmits++
	}, t.Name())
	defer lifecycle.SecurityACMEUpdateSignal.RemoveListener(t.Name())

	lifecycle.NetworkUpdateSignal.AddListener(func(_ context.Context, n system.Network) {
		networkEmits = append(networkEmits, n.RestServerAddress)
	}, t.Name())
	defer lifecycle.NetworkUpdateSignal.RemoveListener(t.Name())

	// Partial ACME payload: challenge/address/ca_url are filled by normalize.
	put := system.SecurityPut{
		ACME: system.SecurityACME{
			Domain:   "example.com",
			Email:    "admin@example.com",
			AgreeTOS: true,
		},
		TrustedTLSClientCertFingerprints: []string{"fingerprint"},
	}

	require.NoError(t, UpdateSecurity(t.Context(), put))
	require.Equal(t, 1, acmeEmits)

	// Repeating the exact same request must not look like an ACME change.
	require.NoError(t, UpdateSecurity(t.Context(), put))
	require.Equal(t, 1, acmeEmits, "identical security PUT triggered an ACME renewal")

	require.NoError(t, UpdateNetwork(t.Context(), system.NetworkPut{
		RestServerAddress:       "0.0.0.0",
		OperationsCenterAddress: "https://localhost:7443",
	}))
	require.Equal(t, []string{"0.0.0.0:7443"}, networkEmits, "listeners got the un-normalized address")

	// Re-submitting the network config applies it again, so an operator can
	// recover a listener which failed to pick up the previous update.
	require.NoError(t, UpdateNetwork(t.Context(), system.NetworkPut{
		RestServerAddress:       "0.0.0.0:7443",
		OperationsCenterAddress: "https://localhost:7443",
	}))
	require.Len(t, networkEmits, 2)

	// An update of an unrelated section must leave the network alone.
	require.NoError(t, UpdateSettings(t.Context(), system.SettingsPut{
		LogLevel: "DEBUG",
	}))
	require.Len(t, networkEmits, 2, "an unrelated update disturbed the network config")
}

func Test_notify_componentLogLevels(t *testing.T) {
	InitTest(t, &mock.EnvironmentMock{IsIncusOSFunc: func() bool { return false }}, nil)

	logBuf := &log.SyncBuffer{}

	require.NoError(t, logger.InitLogger(logBuf, "", false, false, true))

	t.Cleanup(func() {
		require.NoError(t, logger.InitLogger(logBuf, "", false, false, true), "the log levels must not leak into other tests")
	})

	ctx := logger.ContextWithComponent(t.Context(), "provisioning.server_repo")

	slog.DebugContext(ctx, "before update")
	require.Empty(t, logBuf.String(), "the default log level WARN suppresses debug records")

	require.NoError(t, UpdateSettings(t.Context(), system.SettingsPut{
		LogLevels: map[string]string{"provisioning": "DEBUG"},
	}))

	slog.DebugContext(ctx, "after update")
	require.Contains(t, logBuf.String(), "after update", "the log level configured for a parent component was not applied")

	require.NoError(t, UpdateSettings(t.Context(), system.SettingsPut{}))

	logBuf.Reset()
	slog.DebugContext(ctx, "after reset")
	require.Empty(t, logBuf.String(), "removing the per component log level did not take effect")
}

func Test_notify_metricsComponents(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T)
		settings system.SettingsPut

		want bool
	}{
		{
			name: "enabled by parent component",
			setup: func(t *testing.T) {
				t.Helper()

				metrics.Apply(false, nil)
			},
			settings: system.SettingsPut{
				MetricsEnabled:    true,
				MetricsComponents: []string{"test"},
			},

			want: true,
		},
		{
			name: "component not listed",
			setup: func(t *testing.T) {
				t.Helper()

				metrics.Apply(true, []string{"test"})
			},
			settings: system.SettingsPut{
				MetricsEnabled:    true,
				MetricsComponents: []string{"other"},
			},

			want: false,
		},
		{
			name: "metrics disabled",
			setup: func(t *testing.T) {
				t.Helper()

				metrics.Apply(true, []string{"test"})
			},
			settings: system.SettingsPut{
				MetricsEnabled:    false,
				MetricsComponents: []string{"test"},
			},

			want: false,
		},
		{
			name: "only enabled changes to true",
			setup: func(t *testing.T) {
				t.Helper()

				err := UpdateSettings(t.Context(), system.SettingsPut{
					MetricsEnabled:    false,
					MetricsComponents: []string{"test"},
				})
				require.NoError(t, err)
			},
			settings: system.SettingsPut{
				MetricsEnabled:    true,
				MetricsComponents: []string{"test"},
			},

			want: true,
		},
		{
			name: "only enabled changes to false",
			setup: func(t *testing.T) {
				t.Helper()

				err := UpdateSettings(t.Context(), system.SettingsPut{
					MetricsEnabled:    true,
					MetricsComponents: []string{"test"},
				})
				require.NoError(t, err)
			},
			settings: system.SettingsPut{
				MetricsEnabled:    false,
				MetricsComponents: []string{"test"},
			},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			InitTest(t, &mock.EnvironmentMock{IsIncusOSFunc: func() bool { return false }}, nil)

			component := metrics.RegisterComponent("test.notify_metrics_components")

			t.Cleanup(func() { metrics.Apply(false, nil) })

			tc.setup(t)
			require.NotEqual(t, tc.want, component.Enabled(), "the test starts in the opposite state")

			// Run test
			err := UpdateSettings(t.Context(), tc.settings)

			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.want, component.Enabled())
		})
	}
}
