package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func Test_serverHandler_serverPut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		ifMatch             string
		serviceGetByNameErr error
		serviceUpdateErr    error

		wantUpdateCalls int
		wantStatusCode  int
	}{
		{
			name: "success",

			wantUpdateCalls: 1,
			wantStatusCode:  http.StatusOK,
		},
		{
			name:                "error - service.GetByName",
			serviceGetByNameErr: boom.Error,

			wantUpdateCalls: 0,
			wantStatusCode:  http.StatusInternalServerError,
		},
		{
			name:    "error - ETag mismatch",
			ifMatch: `"invalid"`,

			wantUpdateCalls: 0,
			wantStatusCode:  http.StatusPreconditionFailed,
		},
		{
			name:             "error - service.Update",
			serviceUpdateErr: boom.Error,

			wantUpdateCalls: 1,
			wantStatusCode:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			service := &provisioningMock.ServerServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					require.False(t, transaction.IsActive(ctx), "the handler must not open a transaction, the update calls the BMC and the server API")

					return &provisioning.Server{Name: name}, tc.serviceGetByNameErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server, force bool, updateSystem bool, bmcConnectionTest bool) error {
					require.False(t, transaction.IsActive(ctx), "the handler must not open a transaction, the update calls the BMC and the server API")
					require.True(t, bmcConnectionTest, "an update through the API must test the BMC connection")

					return tc.serviceUpdateErr
				},
			}

			handler := &serverHandler{
				service: service,
			}

			req := httptest.NewRequest(http.MethodPut, "/one", strings.NewReader(`{}`))
			req.SetPathValue("name", "one")
			req.Header.Set("If-Match", tc.ifMatch)

			rec := httptest.NewRecorder()

			// Run test
			err := handler.serverPut(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			require.Len(t, service.UpdateCalls(), tc.wantUpdateCalls)
			require.Equal(t, tc.wantStatusCode, rec.Code)
		})
	}
}

func Test_serverHandler_serverDeployPostRecordBMC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string

		wantStatusCode  int
		wantDeployCalls int
		wantRecordBMC   bool
	}{
		{
			name: "success - without record_bmc",

			wantStatusCode:  http.StatusOK,
			wantDeployCalls: 1,
			wantRecordBMC:   false,
		},
		{
			name:  "success - with record_bmc",
			query: "?record_bmc=true",

			wantStatusCode:  http.StatusOK,
			wantDeployCalls: 1,
			wantRecordBMC:   true,
		},
		{
			name:  "error - record_bmc invalid",
			query: "?record_bmc=maybe",

			wantStatusCode:  http.StatusBadRequest,
			wantDeployCalls: 0,
		},
	}

	for _, tc := range tests {
		// Not parallel, rendering the shared response.EmptySyncResponse races.
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			service := &provisioningMock.ServerServiceMock{
				DeployByNameFunc: func(ctx context.Context, name string, request provisioning.ServerDeploymentRequest) error {
					return nil
				},
			}

			handler := &serverHandler{
				service: service,
			}

			body := `{"token_uuid": "b32d0079-c48b-4957-b1cb-bef54125c861", "seed": "default"}`
			req := httptest.NewRequest(http.MethodPost, "/one/:deploy"+tc.query, strings.NewReader(body))
			req.SetPathValue("name", "one")

			rec := httptest.NewRecorder()

			// Run test
			err := handler.serverDeployPost(req).Render(rec)

			// Assert
			require.NoError(t, err, "rendering the response must not fail")
			require.Equal(t, tc.wantStatusCode, rec.Code, "body: %s", rec.Body.String())
			require.Len(t, service.DeployByNameCalls(), tc.wantDeployCalls)

			for _, call := range service.DeployByNameCalls() {
				require.Equal(t, tc.wantRecordBMC, call.Request.RecordBMC, "record_bmc is passed to the service")
			}
		})
	}
}
