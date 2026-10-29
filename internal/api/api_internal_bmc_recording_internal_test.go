package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/shared/api"
)

// entitlementAuthorizer records the checked entitlements and denies all of
// them, if deny is set.
type entitlementAuthorizer struct {
	deny bool

	mu           sync.Mutex
	entitlements []authz.Entitlement
}

func (a *entitlementAuthorizer) CheckPermission(ctx context.Context, details *authz.RequestDetails, object authz.Object, entitlement authz.Entitlement) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.entitlements = append(a.entitlements, entitlement)

	if a.deny {
		return api.StatusErrorf(http.StatusForbidden, "Permission denied")
	}

	return nil
}

func Test_internalBMCRecordingHandler(t *testing.T) {
	t.Parallel()

	const id = "20261027T081500Z"

	tests := []struct {
		name       string
		method     string
		target     string
		deny       bool
		serviceErr error

		wantStatus             int
		wantBody               string
		wantContentType        string
		wantContentDisposition string
		wantCompressed         bool
		wantArchiveAnonymize   []bool
		wantDeleteCalls        int
	}{
		{
			name:   "success - list",
			method: http.MethodGet,
			target: "/bmc-recordings/one",

			wantStatus:      http.StatusOK,
			wantBody:        `"recording_id":"` + id + `"`,
			wantContentType: "application/json",
		},
		{
			name:       "error - list of an unknown server",
			method:     http.MethodGet,
			target:     "/bmc-recordings/one",
			serviceErr: domain.NewErrorf(domain.ErrNotFound, "", "Server not found"),

			wantStatus:      http.StatusNotFound,
			wantBody:        "Server not found",
			wantContentType: "application/json",
		},
		{
			name:   "success - manifest",
			method: http.MethodGet,
			target: "/bmc-recordings/one/" + id,

			wantStatus:      http.StatusOK,
			wantBody:        `"server":"one"`,
			wantContentType: "application/json",
		},
		{
			name:   "success - archive",
			method: http.MethodGet,
			target: "/bmc-recordings/one/" + id + "?archive=zip",

			wantStatus:             http.StatusOK,
			wantBody:               "zip content",
			wantContentType:        "application/zip",
			wantContentDisposition: `attachment; filename="one-` + id + `.zip"`,
			wantArchiveAnonymize:   []bool{false},
		},
		{
			name:   "success - anonymized archive",
			method: http.MethodGet,
			target: "/bmc-recordings/one/" + id + "?archive=zip&anonymize=true",

			wantStatus:             http.StatusOK,
			wantBody:               "zip content",
			wantContentType:        "application/zip",
			wantContentDisposition: `attachment; filename="one-` + id + `.zip"`,
			wantArchiveAnonymize:   []bool{true},
		},
		{
			name:   "error - archive type not supported",
			method: http.MethodGet,
			target: "/bmc-recordings/one/" + id + "?archive=tar",

			wantStatus:      http.StatusBadRequest,
			wantBody:        `Archive type \"tar\" not supported`,
			wantContentType: "application/json",
		},
		{
			name:   "error - anonymize invalid",
			method: http.MethodGet,
			target: "/bmc-recordings/one/" + id + "?archive=zip&anonymize=maybe",

			wantStatus:      http.StatusBadRequest,
			wantBody:        "anonymize",
			wantContentType: "application/json",
		},
		{
			name:   "success - exchanges file",
			method: http.MethodGet,
			target: "/bmc-recordings/one/" + id + "/exchanges.jsonl",

			wantStatus:             http.StatusOK,
			wantBody:               "exchanges.jsonl",
			wantContentType:        "application/x-ndjson",
			wantContentDisposition: `attachment; filename="exchanges.jsonl"`,
			wantCompressed:         true,
		},
		{
			name:   "success - manifest file",
			method: http.MethodGet,
			target: "/bmc-recordings/one/" + id + "/manifest.json",

			wantStatus:             http.StatusOK,
			wantBody:               "manifest.json",
			wantContentType:        "application/json; charset=utf-8",
			wantContentDisposition: `attachment; filename="manifest.json"`,
			wantCompressed:         true,
		},
		{
			name:       "error - file not allowed",
			method:     http.MethodGet,
			target:     "/bmc-recordings/one/" + id + "/other.txt",
			serviceErr: domain.NewValidationErrf("Invalid BMC recording file"),

			wantStatus:      http.StatusBadRequest,
			wantBody:        "Invalid BMC recording file",
			wantContentType: "application/json",
		},
		{
			name:   "success - delete",
			method: http.MethodDelete,
			target: "/bmc-recordings/one/" + id,

			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantDeleteCalls: 1,
		},
		{
			name:   "error - no admin permission",
			method: http.MethodDelete,
			target: "/bmc-recordings/one/" + id,
			deny:   true,

			wantStatus:      http.StatusForbidden,
			wantBody:        "Permission denied",
			wantContentType: "application/json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			service := &provisioningMock.ServerServiceMock{
				GetBMCRecordingsFunc: func(ctx context.Context, name string) ([]api.BMCRecordingManifest, error) {
					return []api.BMCRecordingManifest{{Server: name, RecordingID: id}}, tc.serviceErr
				},
				GetBMCRecordingFunc: func(ctx context.Context, name string, id string) (*api.BMCRecordingManifest, error) {
					return &api.BMCRecordingManifest{Server: name, RecordingID: id}, tc.serviceErr
				},
				GetBMCRecordingArchiveFunc: func(ctx context.Context, name string, id string, anonymize bool) (io.ReadCloser, int, error) {
					return io.NopCloser(strings.NewReader("zip content")), len("zip content"), tc.serviceErr
				},
				GetBMCRecordingFileFunc: func(ctx context.Context, name string, id string, filename string) (io.ReadCloser, int64, error) {
					return io.NopCloser(strings.NewReader(filename)), int64(len(filename)), tc.serviceErr
				},
				DeleteBMCRecordingFunc: func(ctx context.Context, name string, id string) error {
					return tc.serviceErr
				},
			}

			authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

			serveMux := http.NewServeMux()
			router := newRouter(serveMux).AddMiddlewares(
				authenticator.Middleware(),
			)

			recordingAuthorizer := &entitlementAuthorizer{deny: tc.deny}

			var authorizer authz.Authorizer = recordingAuthorizer
			registerInternalBMCRecordingHandler(router, &authorizer, service)

			server := httptest.NewServer(serveMux)
			t.Cleanup(server.Close)

			req, err := http.NewRequestWithContext(t.Context(), tc.method, server.URL+tc.target, http.NoBody)
			require.NoError(t, err)

			// The transport asks for gzip and decompresses transparently.
			client := &http.Client{Transport: &http.Transport{}}

			// Run test
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Assert
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, tc.wantStatus, resp.StatusCode, "body: %s", body)
			require.Contains(t, string(body), tc.wantBody)
			require.Equal(t, tc.wantContentType, resp.Header.Get("Content-Type"))
			require.Equal(t, tc.wantContentDisposition, resp.Header.Get("Content-Disposition"))
			require.Equal(t, tc.wantCompressed, resp.Uncompressed, "the files are compressed for the transfer, the zip is not compressed again")
			require.Equal(t, []authz.Entitlement{authz.EntitlementCanAdmin}, recordingAuthorizer.entitlements, "every route requires admin")

			var anonymize []bool
			for _, call := range service.GetBMCRecordingArchiveCalls() {
				anonymize = append(anonymize, call.Anonymize)
			}

			require.Equal(t, tc.wantArchiveAnonymize, anonymize, "anonymize is forwarded")
			require.Len(t, service.DeleteBMCRecordingCalls(), tc.wantDeleteCalls)
		})
	}
}
