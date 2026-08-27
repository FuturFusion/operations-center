package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/security/authz"
	systemMock "github.com/FuturFusion/operations-center/internal/system/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_systemHandler_certificateGet(t *testing.T) {
	tests := []struct {
		name                           string
		systemServiceGetCertificate    apisystem.Certificate
		systemServiceGetCertificateErr error

		wantStatus              int
		wantResponseBodyContain string
	}{
		{
			name: "success",
			systemServiceGetCertificate: apisystem.Certificate{
				Certificate: "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
				Fingerprint: "fd200419b271f1dc2a5591b693cc5774b7f234e1ff8c6b78ad703b6888fe2b69",
			},

			wantStatus:              http.StatusOK,
			wantResponseBodyContain: "fd200419b271f1dc2a5591b693cc5774b7f234e1ff8c6b78ad703b6888fe2b69",
		},
		{
			name:                           "error - systemService.GetCertificate",
			systemServiceGetCertificateErr: boom.Error,

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
				GetCertificateFunc: func(ctx context.Context) (apisystem.Certificate, error) {
					return tc.systemServiceGetCertificate, tc.systemServiceGetCertificateErr
				},
			}

			var authorizer authz.Authorizer = noopAuthorizer{}
			registerSystemHandler(router, &authorizer, systemService)

			server := httptest.NewServer(serveMux)
			t.Cleanup(func() {
				server.Close()
			})

			// Execute http request
			req, err := http.NewRequest(http.MethodGet, server.URL+"/certificate", nil)
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

			// The private key is never part of the response.
			require.NotContains(t, string(body), "PRIVATE KEY")
		})
	}
}

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
