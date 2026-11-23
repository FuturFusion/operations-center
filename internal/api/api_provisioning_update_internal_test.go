package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func Test_updateHandler_updateArchiveGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		target               string
		uuid                 string
		serviceGetArchive    string
		serviceGetArchiveErr error

		wantGetArchiveCalls     int
		wantStatusCode          int
		wantContentLength       string
		wantResponseBodyContain string
	}{
		{
			name:                 "success",
			target:               "/" + updateArchiveTestUUID + "/archive",
			uuid:                 updateArchiveTestUUID,
			serviceGetArchive:    "archive content",
			serviceGetArchiveErr: nil,

			wantGetArchiveCalls:     1,
			wantStatusCode:          http.StatusOK,
			wantContentLength:       "15",
			wantResponseBodyContain: "archive content",
		},
		{
			name:                 "error - invalid uuid",
			target:               "/invalid/archive",
			uuid:                 "invalid",
			serviceGetArchive:    "",
			serviceGetArchiveErr: nil,

			wantGetArchiveCalls:     0,
			wantStatusCode:          http.StatusBadRequest,
			wantContentLength:       "",
			wantResponseBodyContain: `"error_code":400`,
		},
		{
			name:                 "error - service.GetArchive",
			target:               "/" + updateArchiveTestUUID + "/archive",
			uuid:                 updateArchiveTestUUID,
			serviceGetArchive:    "",
			serviceGetArchiveErr: boom.Error,

			wantGetArchiveCalls:     1,
			wantStatusCode:          http.StatusInternalServerError,
			wantContentLength:       "",
			wantResponseBodyContain: `"error_code":500`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			service := &provisioningMock.UpdateServiceMock{
				GetArchiveFunc: func(ctx context.Context, id uuid.UUID) (io.ReadCloser, int, error) {
					return io.NopCloser(strings.NewReader(tc.serviceGetArchive)), len(tc.serviceGetArchive), tc.serviceGetArchiveErr
				},
			}

			handler := &updateHandler{
				service: service,
			}

			req := httptest.NewRequest(http.MethodGet, tc.target, http.NoBody)
			req.SetPathValue("uuid", tc.uuid)
			rec := httptest.NewRecorder()

			// Run test
			err := handler.updateArchiveGet(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			require.Len(t, service.GetArchiveCalls(), tc.wantGetArchiveCalls, "unexpected number of get archive calls")
			require.Equal(t, tc.wantStatusCode, rec.Code, "unexpected status code")
			require.Equal(t, tc.wantContentLength, rec.Header().Get("Content-Length"), "unexpected content length")
			require.Contains(t, rec.Body.String(), tc.wantResponseBodyContain, "unexpected response body")
		})
	}
}

const updateArchiveTestUUID = "3b9d0f85-67b4-480e-b369-fef25e9d8ccc"

func Test_updateHandler_updatesDownloadPost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		body               string
		serviceDownloadErr error

		wantVersions            []string
		wantChannels            [][]string
		wantStatusCode          int
		wantResponseBodyContain string
	}{
		{
			name:               "success",
			body:               `{"version":"202610272234","channels":["legacy"]}`,
			serviceDownloadErr: nil,

			wantVersions:            []string{"202610272234"},
			wantChannels:            [][]string{{"legacy"}},
			wantStatusCode:          http.StatusCreated,
			wantResponseBodyContain: "/1.0/provisioning/updates/" + updateArchiveTestUUID,
		},
		{
			name:               "error - invalid body",
			body:               `{`,
			serviceDownloadErr: nil,

			wantVersions:            nil,
			wantChannels:            nil,
			wantStatusCode:          http.StatusBadRequest,
			wantResponseBodyContain: `"error_code":400`,
		},
		{
			name:               "error - service.Download",
			body:               `{"version":"202610272234"}`,
			serviceDownloadErr: boom.Error,

			wantVersions:            []string{"202610272234"},
			wantChannels:            [][]string{nil},
			wantStatusCode:          http.StatusInternalServerError,
			wantResponseBodyContain: `"error_code":500`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			service := &provisioningMock.UpdateServiceMock{
				DownloadFunc: func(ctx context.Context, version string, channels []string) (uuid.UUID, error) {
					return uuid.MustParse(updateArchiveTestUUID), tc.serviceDownloadErr
				},
			}

			handler := &updateHandler{
				service: service,
			}

			req := httptest.NewRequest(http.MethodPost, "/:download", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			// Run test
			err := handler.updatesDownloadPost(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			var gotVersions []string
			var gotChannels [][]string
			for _, call := range service.DownloadCalls() {
				gotVersions = append(gotVersions, call.Version)
				gotChannels = append(gotChannels, call.Channels)
			}

			require.Equal(t, tc.wantVersions, gotVersions, "unexpected versions requested")
			require.Equal(t, tc.wantChannels, gotChannels, "unexpected channels requested")
			require.Equal(t, tc.wantStatusCode, rec.Code, "unexpected status code")
			require.Contains(t, rec.Body.String()+rec.Header().Get("Location"), tc.wantResponseBodyContain, "unexpected response")
		})
	}
}
