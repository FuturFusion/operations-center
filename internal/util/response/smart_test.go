package response_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestSmartError(t *testing.T) {
	tt := []struct {
		name string
		err  error

		wantCode    int
		wantMessage string
		wantReason  api.ErrorReason
		wantHint    string
		wantDetails map[string]string
	}{
		{
			name: "domain validation error",
			err:  domain.NewValidationErrf("foobar"),

			wantCode:    http.StatusBadRequest,
			wantMessage: "foobar",
			wantReason:  api.ErrorReasonInvalidArgument,
		},
		{
			name: "domain error with reason, hint and details",
			err: fmt.Errorf("Failed to delete server %q: %w", "one",
				domain.NewErrorf(domain.ErrOperationNotPermitted, "server_is_cluster_member", "Server %q is a member of cluster %q", "one", "cluster").
					WithHintf("Remove the server from cluster %q first.", "cluster").
					WithDetail("server", "one").
					WithCause(errors.New("boom!")),
			),

			wantCode:    http.StatusBadRequest,
			wantMessage: `Server "one" is a member of cluster "cluster"`,
			wantReason:  api.ErrorReason("server_is_cluster_member"),
			wantHint:    `Remove the server from cluster "cluster" first.`,
			wantDetails: map[string]string{"server": "one"},
		},
		{
			name: "reason pins the status code",
			err: domain.NewErrorf(domain.ErrConstraintViolation, api.ErrorReasonInsufficientStorage, "Not enough space available in the files repository").
				WithHintf("Free space in the files repository.").
				WithDetail("required_bytes", "100"),

			wantCode:    http.StatusInsufficientStorage,
			wantMessage: "Not enough space available in the files repository",
			wantReason:  api.ErrorReasonInsufficientStorage,
			wantHint:    "Free space in the files repository.",
			wantDetails: map[string]string{"required_bytes": "100"},
		},
		{
			name: "domain error of kind not found",
			err:  domain.NewErrorf(domain.ErrNotFound, "", "Server %q not found", "one"),

			wantCode:    http.StatusNotFound,
			wantMessage: `Server "one" not found`,
			wantReason:  api.ErrorReasonNotFound,
		},
		{
			name: "wrapped not found",
			err:  fmt.Errorf("Failed to get server %q: %w", "one", domain.ErrNotFound),

			wantCode:    http.StatusNotFound,
			wantMessage: `Failed to get server "one"`,
			wantReason:  api.ErrorReasonNotFound,
		},
		{
			name: "wrapped operation not permitted",
			err:  fmt.Errorf("Server %q is clustered: %w", "one", domain.ErrOperationNotPermitted),

			wantCode:    http.StatusBadRequest,
			wantMessage: `Server "one" is clustered`,
			wantReason:  api.ErrorReasonOperationNotPermitted,
		},
		{
			name: "constraint violation",
			err:  fmt.Errorf("Failed to create server: %w", domain.ErrConstraintViolation),

			wantCode:    http.StatusBadRequest,
			wantMessage: "Failed to create server",
			wantReason:  api.ErrorReasonConstraintViolation,
		},
		{
			name: "not authenticated",
			err:  fmt.Errorf("Failed to authenticate: %w", domain.ErrNotAuthenticated),

			wantCode:    http.StatusUnauthorized,
			wantMessage: "Failed to authenticate",
			wantReason:  api.ErrorReasonUnauthenticated,
		},
		{
			name: "not authorized",
			err:  fmt.Errorf("Access denied: %w", domain.ErrNotAuthorized),

			wantCode:    http.StatusForbidden,
			wantMessage: "Access denied",
			wantReason:  api.ErrorReasonForbidden,
		},
		{
			name: "status error",
			err:  api.StatusErrorf(http.StatusPreconditionFailed, "ETag doesn't match"),

			wantCode:    http.StatusPreconditionFailed,
			wantMessage: "ETag doesn't match",
			wantReason:  api.ErrorReasonPreconditionFailed,
		},
		{
			name: "retryable error",
			err:  domain.NewRetryableErr(errors.New("Unable to connect to: 1.2.3.4")),

			wantCode:    http.StatusServiceUnavailable,
			wantMessage: "Unable to connect to: 1.2.3.4",
			wantReason:  api.ErrorReasonUnavailable,
		},
		{
			name: "error reported by an incus server",
			err:  fmt.Errorf("Failed to fetch instances: %w", incusapi.StatusErrorf(http.StatusNotFound, "Instance not found")),

			wantCode:    http.StatusBadGateway,
			wantMessage: "Failed to fetch instances: Instance not found",
			wantReason:  api.ErrorReasonUpstream,
		},
		{
			name: "wrapped sql no rows",
			err:  fmt.Errorf("Failed to fetch server: %w", sql.ErrNoRows),

			wantCode:    http.StatusNotFound,
			wantMessage: "Failed to fetch server: sql: no rows in result set",
			wantReason:  api.ErrorReasonNotFound,
		},
		{
			name: "bare os not exist",
			err:  os.ErrNotExist,

			wantCode:    http.StatusNotFound,
			wantMessage: "Not Found",
			wantReason:  api.ErrorReasonNotFound,
		},
		{
			name: "unclassified error",
			err:  fmt.Errorf("Failed to fetch from %q table: %w", "servers", errors.New("no such column: name")),

			wantCode:    http.StatusInternalServerError,
			wantMessage: response.InternalErrorMessage,
			wantReason:  api.ErrorReasonInternal,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			resp := response.SmartError(tc.err)

			require.Equal(t, tc.wantCode, resp.Code())
			require.Equal(t, tc.wantMessage, resp.String(), "the message reported to the user")

			recorder := httptest.NewRecorder()
			recorder.Header().Set(api.RequestIDHeader, "request-id")

			err := resp.Render(recorder)
			require.NoError(t, err)

			var body api.ResponseRaw
			err = json.NewDecoder(recorder.Body).Decode(&body)
			require.NoError(t, err)

			require.Equal(t, api.ErrorResponse, body.Type)
			require.Equal(t, tc.wantCode, body.Code)
			require.Equal(t, tc.wantMessage, body.Error)

			metadata := errorMetadata(t, body.Metadata)

			require.Equal(t, tc.wantReason, metadata.Reason)
			require.Equal(t, tc.wantHint, metadata.Hint, "the hint tells the user how to resolve the error")
			require.Equal(t, tc.wantDetails, metadata.Details)
			require.Equal(t, "request-id", metadata.RequestID, "the request ID allows to find the error in the log")
		})
	}
}

func TestSmartError_nil(t *testing.T) {
	require.Equal(t, response.EmptySyncResponse, response.SmartError(nil), "no error is reported as empty sync response")
}

func TestErrorResponseFromError(t *testing.T) {
	resp := response.BadRequest(fmt.Errorf("Invalid request body: %w", errors.New("unexpected EOF")))

	require.Equal(t, http.StatusBadRequest, resp.Code())
	require.Equal(t, "Invalid request body: unexpected EOF", resp.String(), "the status text is not repeated in the message")

	recorder := httptest.NewRecorder()

	err := resp.Render(recorder)
	require.NoError(t, err)

	var body api.ResponseRaw
	err = json.NewDecoder(recorder.Body).Decode(&body)
	require.NoError(t, err)

	require.Equal(t, api.ErrorReasonInvalidArgument, errorMetadata(t, body.Metadata).Reason)
}

func errorMetadata(t *testing.T, metadata any) api.ErrorMetadata {
	t.Helper()

	raw, err := json.Marshal(metadata)
	require.NoError(t, err)

	var errorMetadata api.ErrorMetadata
	err = json.Unmarshal(raw, &errorMetadata)
	require.NoError(t, err)

	return errorMetadata
}
