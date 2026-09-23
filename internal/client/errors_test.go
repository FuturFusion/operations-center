package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerError(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		contentType string
		body        string

		wantMessage   string
		wantReason    api.ErrorReason
		wantHint      string
		wantDetails   map[string]string
		wantRequestID string
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name:        "not found",
			statusCode:  http.StatusNotFound,
			contentType: "application/json",
			body:        `{"type":"error","error_code":404,"error":"Server \"one\" not found","metadata":{"reason":"not_found","request_id":"request-id"}}`,

			wantMessage:   `Server "one" not found`,
			wantReason:    api.ErrorReasonNotFound,
			wantRequestID: "request-id",
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotFound, a...)
			},
		},
		{
			name:        "not authenticated",
			statusCode:  http.StatusUnauthorized,
			contentType: "application/json",
			body:        `{"type":"error","error_code":401,"error":"Failed to authenticate","metadata":{"reason":"unauthenticated"}}`,

			wantMessage: "Failed to authenticate",
			wantReason:  api.ErrorReasonUnauthenticated,
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated, a...)
			},
		},
		{
			name:        "not authorized",
			statusCode:  http.StatusForbidden,
			contentType: "application/json",
			body:        `{"type":"error","error_code":403,"error":"Access denied","metadata":{"reason":"forbidden"}}`,

			wantMessage: "Access denied",
			wantReason:  api.ErrorReasonForbidden,
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthorized, a...)
			},
		},
		{
			name:        "operation not permitted with details",
			statusCode:  http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"type":"error","error_code":400,"error":"Server \"one\" is a member of cluster \"two\"","metadata":{"reason":"server_is_cluster_member","hint":"Remove the server from the cluster first.","details":{"server":"one","cluster":"two"}}}`,

			wantMessage: `Server "one" is a member of cluster "two"`,
			wantReason:  api.ErrorReason("server_is_cluster_member"),
			wantHint:    "Remove the server from the cluster first.",
			wantDetails: map[string]string{"server": "one", "cluster": "two"},
			assertErr:   require.Error,
		},
		{
			name:        "response without api envelope",
			statusCode:  http.StatusBadGateway,
			contentType: "text/plain",
			body:        "proxy error",

			wantMessage: "502 Bad Gateway",
			assertErr:   require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set(api.RequestIDHeader, "request-id")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))

			t.Cleanup(srv.Close)

			c, err := client.New(srv.URL)
			require.NoError(t, err)

			_, err = c.GetAPIServerInfo(t.Context())

			tc.assertErr(t, err)
			require.Equal(t, tc.wantMessage, err.Error(), "the message of the server is reported as is")

			var serverErr *client.ServerError
			require.ErrorAs(t, err, &serverErr)

			require.Equal(t, tc.statusCode, serverErr.StatusCode)
			require.Equal(t, tc.wantReason, serverErr.Reason)
			require.Equal(t, tc.wantHint, serverErr.Hint, "the hint of the server is reported to the user")
			require.Equal(t, tc.wantDetails, serverErr.Details)
			require.Equal(t, "request-id", serverErr.RequestID, "the request ID allows to find the error in the log of the server")

			require.True(t, api.StatusErrorCheck(err, tc.statusCode), "the status code of the response is matched as api.StatusError")
		})
	}
}

func TestServerError_Error(t *testing.T) {
	serverErr := &client.ServerError{StatusCode: http.StatusNotFound}

	require.Equal(t, "Not Found", serverErr.Error(), "an empty message falls back to the status text")
	require.NotErrorIs(t, serverErr, domain.ErrNotAuthorized)
}
