package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/authn"
	"github.com/FuturFusion/operations-center/internal/authz"
	systemMock "github.com/FuturFusion/operations-center/internal/system/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func Test_systemHandler_certificatePut(t *testing.T) {
	tests := []struct {
		name                              string
		requestBody                       string
		systemServiceUpdateCertificateErr error

		wantStatus              int
		wantResponseBodyContain string
	}{
		{
			name:        "success",
			requestBody: `{"certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----", "key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"}`,

			wantStatus:              http.StatusOK,
			wantResponseBodyContain: "Success",
		},
		{
			name:        "error - invalid request body",
			requestBody: `invalid`,

			wantStatus:              http.StatusBadRequest,
			wantResponseBodyContain: "invalid character 'i'",
		},
		{
			name:                              "error - systemService.UpdateCertificate",
			requestBody:                       `{"certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----", "key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"}`,
			systemServiceUpdateCertificateErr: boom.Error,

			wantStatus:              http.StatusInternalServerError,
			wantResponseBodyContain: "boom!",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			authenticator := authn.New([]authn.Auther{dummyAuthenticator{}})

			serveMux := http.NewServeMux()
			router := newRouter(serveMux).AddMiddlewares(
				authenticator.Middleware(),
			)

			systemService := &systemMock.SystemServiceMock{
				UpdateCertificateFunc: func(ctx context.Context, cert, key string) error {
					return tc.systemServiceUpdateCertificateErr
				},
			}

			var authorizer authz.Authorizer = noopAuthorizer{}
			registerSystemHandler(router, &authorizer, systemService)

			server := httptest.NewServer(serveMux)
			t.Cleanup(func() {
				server.Close()
			})

			// Execute http request
			req, err := http.NewRequest(http.MethodPost, server.URL+"/certificate", bytes.NewBufferString(tc.requestBody))
			require.NoError(t, err)

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Assert results
			require.Equal(t, tc.wantStatus, resp.StatusCode)
			require.Contains(t, string(body), tc.wantResponseBodyContain)
		})
	}
}
