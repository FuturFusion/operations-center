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
