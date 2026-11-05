package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_ServerBMCRecording(t *testing.T) {
	const id = "20261027T081500Z"

	tests := []struct {
		name         string
		call         func(ctx context.Context, c client.OperationsCenterClient) (any, error)
		responseCode int
		responseBody string

		assertErr  require.ErrorAssertionFunc
		wantMethod string
		wantPath   string
		wantQuery  string
		want       any
	}{
		{
			name: "success - GetServerBMCRecordings",
			call: func(ctx context.Context, c client.OperationsCenterClient) (any, error) {
				return c.GetServerBMCRecordings(ctx, "one")
			},
			responseCode: http.StatusOK,
			responseBody: `{"type":"sync","status":"Success","status_code":200,"metadata":[{"server":"one","recording_id":"` + id + `"}]}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodGet,
			wantPath:   "/internal/bmc-recordings/one",
			want:       []api.BMCRecordingManifest{{Server: "one", RecordingID: id}},
		},
		{
			name: "success - GetServerBMCRecording",
			call: func(ctx context.Context, c client.OperationsCenterClient) (any, error) {
				return c.GetServerBMCRecording(ctx, "one", id)
			},
			responseCode: http.StatusOK,
			responseBody: `{"type":"sync","status":"Success","status_code":200,"metadata":{"server":"one","recording_id":"` + id + `"}}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodGet,
			wantPath:   "/internal/bmc-recordings/one/" + id,
			want:       &api.BMCRecordingManifest{Server: "one", RecordingID: id},
		},
		{
			name: "success - GetServerBMCRecordingArchive",
			call: func(ctx context.Context, c client.OperationsCenterClient) (any, error) {
				rc, err := c.GetServerBMCRecordingArchive(ctx, "one", id, true)
				if err != nil {
					return nil, err
				}

				defer rc.Close()

				body, err := io.ReadAll(rc)

				return string(body), err
			},
			responseCode: http.StatusOK,
			responseBody: "zip content",

			assertErr:  require.NoError,
			wantMethod: http.MethodGet,
			wantPath:   "/internal/bmc-recordings/one/" + id,
			wantQuery:  "anonymize=true&archive=zip",
			want:       "zip content",
		},
		{
			name: "error - GetServerBMCRecordingArchive",
			call: func(ctx context.Context, c client.OperationsCenterClient) (any, error) {
				return c.GetServerBMCRecordingArchive(ctx, "one", id, false)
			},
			responseCode: http.StatusNotFound,
			responseBody: `{"type":"error","error":"not found","error_code":404}`,

			assertErr:  require.Error,
			wantMethod: http.MethodGet,
			wantPath:   "/internal/bmc-recordings/one/" + id,
			wantQuery:  "anonymize=false&archive=zip",
			want:       io.ReadCloser(nil),
		},
		{
			name: "success - DeleteServerBMCRecording",
			call: func(ctx context.Context, c client.OperationsCenterClient) (any, error) {
				return nil, c.DeleteServerBMCRecording(ctx, "one", id)
			},
			responseCode: http.StatusOK,
			responseBody: `{"type":"sync","status":"Success","status_code":200}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodDelete,
			wantPath:   "/internal/bmc-recordings/one/" + id,
		},
		{
			name: "success - DeployServer with BMC recording",
			call: func(ctx context.Context, c client.OperationsCenterClient) (any, error) {
				return nil, c.DeployServer(ctx, "one", api.ServerDeploymentPost{}, client.WithBMCRecording())
			},
			responseCode: http.StatusOK,
			responseBody: `{"type":"sync","status":"Success","status_code":200}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodPost,
			wantPath:   "/1.0/provisioning/servers/one/:deploy",
			wantQuery:  "record_bmc=true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var gotMethod, gotPath, gotQuery string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.EscapedPath()
				gotQuery = r.URL.RawQuery

				w.WriteHeader(tc.responseCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			t.Cleanup(server.Close)

			ocClient, err := client.New(server.URL)
			require.NoError(t, err)

			// Run test
			got, err := tc.call(t.Context(), ocClient)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantMethod, gotMethod)
			require.Equal(t, tc.wantPath, gotPath)
			require.Equal(t, tc.wantQuery, gotQuery)
		})
	}
}

func Test_ServerBMCRecordingRoutes(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name string
		call func(ctx context.Context, c client.OperationsCenterClient, name string) error
	}{
		{"GetServerBMCRecordings", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCRecordings(ctx, n)
			return err
		}},
		{"GetServerBMCRecording", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCRecording(ctx, n, "20261027T081500Z")
			return err
		}},
		{"GetServerBMCRecordingArchive", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			_, err := c.GetServerBMCRecordingArchive(ctx, n, "20261027T081500Z", false)
			return err
		}},
		{"DeleteServerBMCRecording", func(ctx context.Context, c client.OperationsCenterClient, n string) error {
			return c.DeleteServerBMCRecording(ctx, n, "20261027T081500Z")
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Run test
			unauthorizedErr := tc.call(t.Context(), d.unauthorizedHTTPClient, "unknown")
			notFoundErr := tc.call(t.Context(), d.socketClient, "unknown")

			// Assert
			require.ErrorIs(t, unauthorizedErr, domain.ErrNotAuthenticated)
			require.ErrorIs(t, notFoundErr, domain.ErrNotFound, "an unknown server has no recordings")
		})
	}
}
