package client_test

import (
	"context"
	"io"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_GetPprof(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name     string
		client   client.OperationsCenterClient
		settings system.SettingsPut

		assertErr require.ErrorAssertionFunc
		wantBody  string
	}{
		{
			name:     "success",
			client:   d.authorizedHTTPClient,
			settings: system.SettingsPut{PprofEnabled: true},

			assertErr: require.NoError,
			wantBody:  "goroutine profile:",
		},
		{
			name:     "error - not authenticated",
			client:   d.unauthorizedHTTPClient,
			settings: system.SettingsPut{PprofEnabled: true},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated, a...)
			},
		},
		{
			name:     "error - pprof disabled",
			client:   d.authorizedHTTPClient,
			settings: system.SettingsPut{},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "The pprof debug endpoints are disabled", a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			err := d.socketClient.UpdateSystemSettingsConfig(t.Context(), system.SettingsFullPut{SettingsPut: tc.settings})
			require.NoError(t, err)

			t.Cleanup(func() {
				require.NoError(t, d.socketClient.UpdateSystemSettingsConfig(context.Background(), system.SettingsFullPut{}))
			})

			// Run test
			var body []byte
			profile, err := tc.client.GetPprof(t.Context(), "goroutine", url.Values{"debug": []string{"1"}})
			if profile != nil {
				body, err = io.ReadAll(profile)
				require.NoError(t, profile.Close())
			}

			// Assert
			tc.assertErr(t, err)
			require.Contains(t, string(body), tc.wantBody, "profile in text format")
		})
	}
}
