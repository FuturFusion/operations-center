package api

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_debugHandler_pprof(t *testing.T) {
	tests := []struct {
		name         string
		pprofEnabled bool
		target       string

		wantStatus              int
		wantResponseBodyContain string
		wantGzip                bool
	}{
		{
			name:         "success - index",
			pprofEnabled: true,
			target:       "/pprof/",

			wantStatus:              http.StatusOK,
			wantResponseBodyContain: "goroutine?debug=2",
		},
		{
			name:         "success - heap",
			pprofEnabled: true,
			target:       "/pprof/heap",

			wantStatus: http.StatusOK,
			wantGzip:   true,
		},
		{
			name:         "success - profile",
			pprofEnabled: true,
			target:       "/pprof/profile?seconds=1",

			wantStatus: http.StatusOK,
			wantGzip:   true,
		},
		{
			name:         "success - goroutine text",
			pprofEnabled: true,
			target:       "/pprof/goroutine?debug=1",

			wantStatus:              http.StatusOK,
			wantResponseBodyContain: "goroutine profile:",
		},
		{
			name:         "error - disabled",
			pprofEnabled: false,
			target:       "/pprof/heap",

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: "pprof_enabled",
		},
		{
			name:         "error - unknown profile",
			pprofEnabled: true,
			target:       "/pprof/invalid",

			wantStatus:              http.StatusNotFound,
			wantResponseBodyContain: `Unknown pprof profile \"invalid\"`,
		},
		{
			name:         "error - seconds exceeding the maximum",
			pprofEnabled: true,
			target:       "/pprof/profile?seconds=301",

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: "seconds",
		},
		{
			name:         "error - seconds invalid",
			pprofEnabled: true,
			target:       "/pprof/heap?seconds=abc",

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: "seconds",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t, &mock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}, nil)
			err := config.UpdateSettings(context.Background(), apisystem.SettingsPut{
				LogLevel:     "WARN",
				PprofEnabled: tc.pprofEnabled,
			})
			require.NoError(t, err)

			authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

			serveMux := http.NewServeMux()
			router := newRouter(serveMux).AddMiddlewares(
				authenticator.Middleware(),
			)

			var authorizer authz.Authorizer = noopAuthorizer{}
			registerDebugHandler(router, &authorizer)

			server := httptest.NewServer(serveMux)
			t.Cleanup(server.Close)

			// Execute http request
			client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
			resp, err := client.Get(server.URL + tc.target)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Assert
			require.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantGzip {
				gzipReader, err := gzip.NewReader(resp.Body)
				require.NoError(t, err)

				body, err := io.ReadAll(gzipReader)
				require.NoError(t, err)
				require.NotEmpty(t, body)

				return
			}

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Contains(t, string(body), tc.wantResponseBodyContain)
		})
	}
}
