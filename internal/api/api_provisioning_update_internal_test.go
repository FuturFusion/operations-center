package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func Test_updateHandler_updatesRefreshPost(t *testing.T) {
	tests := []struct {
		name              string
		source            string
		target            string
		serviceRefreshErr error

		wantRefreshCalls        int
		wantStatusCode          int
		wantResponseBodyContain string
	}{
		{
			name:              "success - wait",
			source:            "https://updates.example.com",
			target:            "/:refresh?wait=true",
			serviceRefreshErr: nil,

			wantRefreshCalls:        1,
			wantStatusCode:          http.StatusOK,
			wantResponseBodyContain: `"status":"Success"`,
		},
		{
			name:              "error - wait - service.Refresh",
			source:            "https://updates.example.com",
			target:            "/:refresh?wait=true",
			serviceRefreshErr: boom.Error,

			wantRefreshCalls:        1,
			wantStatusCode:          http.StatusInternalServerError,
			wantResponseBodyContain: `"error_code":500`,
		},
		{
			name:              "error - wait - no update source configured",
			source:            "",
			target:            "/:refresh?wait=true",
			serviceRefreshErr: nil,

			wantRefreshCalls:        0,
			wantStatusCode:          http.StatusBadRequest,
			wantResponseBodyContain: "No update source is configured",
		},
		{
			name:              "error - background - no update source configured",
			source:            "",
			target:            "/:refresh",
			serviceRefreshErr: nil,

			wantRefreshCalls:        0,
			wantStatusCode:          http.StatusBadRequest,
			wantResponseBodyContain: "No update source is configured",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}, nil)

			updatesCfg := config.GetUpdates().UpdatesPut
			updatesCfg.Source = tc.source
			err := config.UpdateUpdates(t.Context(), updatesCfg)
			require.NoError(t, err)

			service := &provisioningMock.UpdateServiceMock{
				RefreshFunc: func(ctx context.Context) error {
					return tc.serviceRefreshErr
				},
			}

			handler := &updateHandler{
				service: service,
			}

			req := httptest.NewRequest(http.MethodPost, tc.target, http.NoBody)
			rec := httptest.NewRecorder()

			// Run test
			err = handler.updatesRefreshPost(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			require.Len(t, service.RefreshCalls(), tc.wantRefreshCalls, "unexpected number of refresh calls")
			require.Equal(t, tc.wantStatusCode, rec.Code, "unexpected status code")
			require.Contains(t, rec.Body.String(), tc.wantResponseBodyContain, "unexpected response body")
		})
	}
}

func Test_updateHandler_updatesDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		target               string
		serviceCleanupErr    error
		serviceCleanupAllErr error

		wantCleanupCalls        int
		wantCleanupAllCalls     int
		wantStatusCode          int
		wantResponseBodyContain string
	}{
		{
			name:                 "success - default",
			target:               "/",
			serviceCleanupErr:    nil,
			serviceCleanupAllErr: nil,

			wantCleanupCalls:        1,
			wantCleanupAllCalls:     0,
			wantStatusCode:          http.StatusOK,
			wantResponseBodyContain: `"status":"Success"`,
		},
		{
			name:                 "success - force=false",
			target:               "/?force=false",
			serviceCleanupErr:    nil,
			serviceCleanupAllErr: nil,

			wantCleanupCalls:        1,
			wantCleanupAllCalls:     0,
			wantStatusCode:          http.StatusOK,
			wantResponseBodyContain: `"status":"Success"`,
		},
		{
			name:                 "success - force=true",
			target:               "/?force=true",
			serviceCleanupErr:    nil,
			serviceCleanupAllErr: nil,

			wantCleanupCalls:        0,
			wantCleanupAllCalls:     1,
			wantStatusCode:          http.StatusOK,
			wantResponseBodyContain: `"status":"Success"`,
		},
		{
			name:                 "error - invalid force",
			target:               "/?force=invalid",
			serviceCleanupErr:    nil,
			serviceCleanupAllErr: nil,

			wantCleanupCalls:        0,
			wantCleanupAllCalls:     0,
			wantStatusCode:          http.StatusBadRequest,
			wantResponseBodyContain: `"error_code":400`,
		},
		{
			name:                 "error - service.Cleanup",
			target:               "/",
			serviceCleanupErr:    boom.Error,
			serviceCleanupAllErr: nil,

			wantCleanupCalls:        1,
			wantCleanupAllCalls:     0,
			wantStatusCode:          http.StatusInternalServerError,
			wantResponseBodyContain: `"error_code":500`,
		},
		{
			name:                 "error - service.CleanupAll",
			target:               "/?force=true",
			serviceCleanupErr:    nil,
			serviceCleanupAllErr: boom.Error,

			wantCleanupCalls:        0,
			wantCleanupAllCalls:     1,
			wantStatusCode:          http.StatusInternalServerError,
			wantResponseBodyContain: `"error_code":500`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			service := &provisioningMock.UpdateServiceMock{
				CleanupFunc: func(ctx context.Context) error {
					return tc.serviceCleanupErr
				},
				CleanupAllFunc: func(ctx context.Context) error {
					return tc.serviceCleanupAllErr
				},
			}

			handler := &updateHandler{
				service: service,
			}

			req := httptest.NewRequest(http.MethodDelete, tc.target, http.NoBody)
			rec := httptest.NewRecorder()

			// Run test
			err := handler.updatesDelete(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			require.Len(t, service.CleanupCalls(), tc.wantCleanupCalls, "unexpected number of cleanup calls")
			require.Len(t, service.CleanupAllCalls(), tc.wantCleanupAllCalls, "unexpected number of cleanup all calls")
			require.Equal(t, tc.wantStatusCode, rec.Code, "unexpected status code")
			require.Contains(t, rec.Body.String(), tc.wantResponseBodyContain, "unexpected response body")
		})
	}
}
