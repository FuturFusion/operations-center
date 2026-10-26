package api_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	"github.com/FuturFusion/operations-center/internal/client"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
	testingopenfga "github.com/FuturFusion/operations-center/internal/util/testing/openfga"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

const oidcCode = `123`

const (
	admin    = "admin"
	operator = "operator"
	user     = "user"
	viewer   = "viewer"
)

var users = []string{
	admin,
	operator,
	user,
	viewer,
}

func TestAuthentication(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Add dummy server.crt.
	f, err := os.Create(filepath.Join(tmpDir, "server.crt"))
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	// Setup client certificate
	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	certFingerprint := incustls.CertFingerprint(cert.Leaf)

	// Setup alternative client certificate
	altCertPEM, altKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	altCert, err := tls.X509KeyPair(altCertPEM, altKeyPEM)
	require.NoError(t, err)

	oidcProvider, accessTokens := setupMockOIDC(t)

	openFGAEndpoint, openFGAStoreID := setupOpenFGA(t)

	var token string

	port := testingnet.FreePort(t, "")
	tcpBaseURL := "https://localhost:" + port

	// Test cases
	type testCase struct {
		name     string
		client   func() *http.Client
		method   string
		resource string
		headers  map[string]string
		body     io.Reader

		wantStatusCode   int
		wantBodyContains string
		wantAuth         string // Only used in tests for /1.0 route.
	}

	tests := []testCase{
		{
			name: "plain http GET /",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "socket GET /",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: "http://unix.socket/",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "plain http GET /1.0 - infos for unauthorized client",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
			wantAuth:       api.AuthenticationUntrusted,
		},
		{
			name: "socket GET /1.0",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: "http://unix.socket/1.0",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
			wantAuth:       api.AuthenticationMethodUnix,
		},
		{
			name: "client cert http GET /1.0",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
			wantAuth:       api.AuthenticationMethodTLS,
		},
		{
			name: "oidc http GET /1.0 as viewer",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
			wantAuth:       api.AuthenticationMethodOIDC,
		},
		{
			name: "plain http GET /1.0/metrics - unauthorized",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/metrics",
			body:     http.NoBody,

			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name: "socket GET /1.0/metrics",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: "http://unix.socket/1.0/metrics",
			body:     http.NoBody,

			wantStatusCode:   http.StatusOK,
			wantBodyContains: "operations_center_build_info",
		},
		{
			name: "client cert http GET /1.0/metrics",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/metrics",
			body:     http.NoBody,

			wantStatusCode:   http.StatusOK,
			wantBodyContains: "operations_center_build_info",
		},
		{
			name: "oidc http GET /1.0/metrics as viewer",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/metrics",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: http.NoBody,

			wantStatusCode:   http.StatusOK,
			wantBodyContains: "operations_center_build_info",
		},
		{
			name: "plain http GET /1.0/provisioning/servers - unauthorized",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			body:     http.NoBody,

			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name: "socket GET /1.0/provisioning/servers",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: "http://unix.socket/1.0/provisioning/servers",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "client cert http GET /1.0/provisioning/servers",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http GET /1.0/provisioning/servers as viewer",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http GET /1.0/provisioning/servers as admin",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http GET /1.0/provisioning/servers as operator",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http POST /1.0/provisioning/tokens as viewer - forbidden",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/tokens",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http POST /1.0/provisioning/tokens as operator - forbidden",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/tokens",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http POST /1.0/provisioning/tokens as admin",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/tokens",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: bytes.NewBufferString(`{
  "uses_remaining": 10,
  "expire_at": "2099-12-31T23:59:59Z"
}`),

			wantStatusCode: http.StatusCreated,
		},

		// Create a server using token based authentication in order to have a server
		// for the subsequent tests.
		{
			name: "token http POST /1.0/provisioning/servers",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/servers?token=TOKEN",
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "channel": "stable"
}`),

			wantStatusCode: http.StatusCreated,
		},
		{
			name: "http POST /1.0/provisioning/servers without token and without authentication",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "channel": "stable"
}`),

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "http POST /1.0/provisioning/servers with invalid token",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/servers?token=invalid",
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "channel": "stable"
}`),

			wantStatusCode: http.StatusBadRequest,
		},
		{
			name: "http POST /1.0/provisioning/servers with unknown token",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/servers?token=01a37f54-4dbd-4d26-a88d-df5a534545fb",
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "channel": "stable"
}`),

			wantStatusCode: http.StatusForbidden,
		},

		{
			name: "oidc http PUT /1.0/provisioning/servers/serverA as viewer",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://viewer:12346/",
  "channel": "stable"
}`),

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http PUT /1.0/provisioning/servers/serverA as operator",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://operator:12346/",
  "channel": "stable"
}`),

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http PUT /1.0/provisioning/servers/serverA as admin",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://admin:12346/",
  "channel": "stable"
}`),

			wantStatusCode: http.StatusOK,
		},

		// PUT /1.0/provisioning/servers/:self is authenticated by the
		// servers own certificate.
		{
			name: "certificate http PUT /1.0/provisioning/servers/:self",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/provisioning/servers/:self",
			body: bytes.NewBufferString(`{
  "connection_url": "https://self-update:12346/"
}`),

			wantStatusCode: http.StatusOK,
		},
		{
			name: "http PUT /1.0/provisioning/servers/:self without certificate",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{}, // No client certificate.
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/provisioning/servers/:self",
			body: bytes.NewBufferString(`{
  "connection_url": "https://self-update:12346/"
}`),

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "http PUT /1.0/provisioning/servers/:self with wrong certificate",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{altCert}, // Wrong client certificate.
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/provisioning/servers/:self",
			body: bytes.NewBufferString(`{
		  "connection_url": "https://self-update:12347/"
		}`),

			wantStatusCode: http.StatusForbidden,
		},

		{
			name: "oidc http DELETE /1.0/provisioning/servers/serverA as viewer",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodDelete,
			resource: tcpBaseURL + "/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http DELETE /1.0/provisioning/servers/serverA as operator",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodDelete,
			resource: tcpBaseURL + "/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http DELETE /1.0/provisioning/servers/serverA as admin",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodDelete,
			resource: tcpBaseURL + "/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
		},

		// ANY /1.0/provisioning/updates routes do not need authentication
		{
			name: "plain http GET /1.0/provisioning/updates",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/updates",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
		},

		// GET /1.0/provisioning/tokens/{token}/seeds/{name} serves the seed
		// config, which needs authentication like any other endpoint. The
		// public flag only relaxes access to the image itself, so it makes no
		// difference here.
		{
			name: "plain http GET /1.0/provisioning/tokens/{token}/seeds/public",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/tokens/TOKEN/seeds/public",
			body:     http.NoBody,

			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name: "plain http GET /1.0/provisioning/tokens/{token}/seeds/private",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/tokens/TOKEN/seeds/private",
			body:     http.NoBody,

			wantStatusCode: http.StatusUnauthorized,
		},
		// GET /1.0/provisioning/tokens/{token}/seeds/public/... (path-based
		// image route) does not need authentication either. It returns 404
		// here because no ready updates exist in this test setup, not
		// because of an authentication failure - the auth bypass check only
		// cares that the request reaches the handler at all.
		{
			name: "plain http GET /1.0/provisioning/tokens/{token}/seeds/public/architecture/x86_64/type/iso/file.iso",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/tokens/TOKEN/seeds/public/architecture/x86_64/type/iso/file.iso",
			body:     http.NoBody,

			wantStatusCode: http.StatusNotFound,
		},
		// GET /1.0/provisioning/tokens/{token}/seeds/private/... (path-based
		// image route) does need authentication, since this seed is created
		// with the public flag set to false during setup.
		{
			name: "plain http GET /1.0/provisioning/tokens/{token}/seeds/private/architecture/x86_64/type/iso/file.iso",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/tokens/TOKEN/seeds/private/architecture/x86_64/type/iso/file.iso",
			body:     http.NoBody,

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "client cert http GET /1.0/system/security",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/system/security",
			body:     http.NoBody,

			wantStatusCode:   http.StatusOK,
			wantBodyContains: openFGAEndpoint,
		},
		{
			name: "oidc http GET /1.0/system/security as admin",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/system/security",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: http.NoBody,

			wantStatusCode:   http.StatusOK,
			wantBodyContains: openFGAEndpoint,
		},
		{
			name: "oidc http PUT /1.0/system/security removing OpenFGA as operator - forbidden",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/system/security",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
				"Content-Type":  "application/json",
			},
			body: bytes.NewBufferString(`{
  "oidc": {
    "issuer": "` + oidcProvider.Issuer() + `",
    "client_id": "` + oidcProvider.ClientID + `",
    "scope": "openid,offline_access,email"
  },
  "trusted_tls_client_cert_fingerprints": [ "` + certFingerprint + `" ]
}`),

			wantStatusCode: http.StatusForbidden,
		},
	}

	// All routes below /1.0/system, /internal/sql and /os require can_admin.
	// The check runs before the handler, so an empty body is enough for the
	// forbidden cases. For admin, the handler runs and rejects most empty
	// bodies. Any status other than 403 proves the access.
	adminRoutes := []struct {
		method string
		path   string

		wantAdminStatusCode int
	}{
		{method: http.MethodGet, path: "/1.0/system/certificate", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodPost, path: "/1.0/system/certificate", wantAdminStatusCode: http.StatusBadRequest},
		{method: http.MethodPost, path: "/1.0/system/certificate/:renew", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodGet, path: "/1.0/system/network", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodPut, path: "/1.0/system/network", wantAdminStatusCode: http.StatusBadRequest},
		{method: http.MethodGet, path: "/1.0/system/security", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodPut, path: "/1.0/system/security", wantAdminStatusCode: http.StatusBadRequest},
		{method: http.MethodGet, path: "/1.0/system/settings", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodPut, path: "/1.0/system/settings", wantAdminStatusCode: http.StatusBadRequest},
		{method: http.MethodGet, path: "/1.0/system/updates", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodPut, path: "/1.0/system/updates", wantAdminStatusCode: http.StatusBadRequest},
		{method: http.MethodPost, path: "/1.0/system/:clean-cache", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodGet, path: "/internal/sql", wantAdminStatusCode: http.StatusOK},
		{method: http.MethodPost, path: "/internal/sql", wantAdminStatusCode: http.StatusBadRequest},
		{method: http.MethodGet, path: "/os/1.0", wantAdminStatusCode: http.StatusBadRequest},
	}

	oidcClient := func() *http.Client {
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	adminRouteTests := make([]testCase, 0, 4*len(adminRoutes))
	for _, route := range adminRoutes {
		for _, role := range []string{operator, user, viewer} {
			adminRouteTests = append(adminRouteTests, testCase{
				name:     fmt.Sprintf("oidc http %s %s as %s - forbidden", route.method, route.path, role),
				client:   oidcClient,
				method:   route.method,
				resource: tcpBaseURL + route.path,
				headers: map[string]string{
					"Authorization": "Bearer " + accessTokens[role],
				},
				body: http.NoBody,

				wantStatusCode: http.StatusForbidden,
			})
		}

		adminRouteTests = append(adminRouteTests, testCase{
			name:     fmt.Sprintf("oidc http %s %s as %s - allowed", route.method, route.path, admin),
			client:   oidcClient,
			method:   route.method,
			resource: tcpBaseURL + route.path,
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: http.NoBody,

			wantStatusCode: route.wantAdminStatusCode,
		})
	}

	tests = slices.Concat(tests, adminRouteTests)

	env := &mock.EnvironmentMock{
		IsIncusOSFunc: func() bool {
			return false
		},
		GetUnixSocketFunc: func() string {
			return filepath.Join(tmpDir, "unix.socket")
		},
		VarDirFunc: func() string {
			return tmpDir
		},
		CacheDirFunc: func() string {
			return tmpDir
		},
		UsrShareDirFunc: func() string {
			return tmpDir
		},
		GetTokenFunc: func(ctx context.Context) (string, error) {
			return "", nil
		},
		ClusteredFunc: func() bool { return false },
	}

	config.InitTest(t, env, nil, config.InternalConfig{
		IsBackgroundTasksDisabled: true,
		SourcePollSkipFirst:       true,
	})
	err = config.UpdateLocalNetwork(ctx, system.NetworkPut{
		RestServerAddress: testingnet.LocalhostIP(t) + ":" + port,
	})
	require.NoError(t, err)

	err = config.UpdateNetwork(ctx, system.NetworkGlobalPut{
		OperationsCenterAddress: "https://127.0.0.1:" + port,
	})
	require.NoError(t, err)
	err = config.UpdateSecurity(ctx, system.SecurityPut{
		TrustedTLSClientCertFingerprints: []string{certFingerprint},
		OIDC: system.SecurityOIDC{
			Issuer:   oidcProvider.Issuer(),
			ClientID: oidcProvider.ClientID,
			Scope:    "openid,offline_access,email",
		},
		OpenFGA: system.SecurityOpenFGA{
			APIURL:   openFGAEndpoint,
			APIToken: "dummy",
			StoreID:  openFGAStoreID,
		},
	})
	require.NoError(t, err)

	// The daemon command applies the metrics settings at startup.
	metrics.Apply(config.GetLocalSettings().MetricsEnabled, config.GetLocalSettings().MetricsComponents)
	t.Cleanup(func() { metrics.Apply(false, nil) })

	d := restapi.NewDaemon(ctx, env)

	err = d.Start(ctx)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

	require.NoError(t, err)

	setupOpenFGATuples(t, openFGAEndpoint, openFGAStoreID)

	token = setupTokenAndSeeds(t, tmpDir)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resource := strings.Replace(tc.resource, "TOKEN", token, 1)
			req, err := http.NewRequest(tc.method, resource, tc.body)
			require.NoError(t, err)

			for key, value := range tc.headers {
				req.Header.Add(key, value)
			}

			resp, err := tc.client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			body := readBody(t, resp.Body)

			require.Equal(t, tc.wantStatusCode, resp.StatusCode, body)
			require.Contains(t, body, tc.wantBodyContains)

			if tc.wantAuth == "" {
				return
			}

			var apiResponse api.Response
			err = json.Unmarshal([]byte(body), &apiResponse)
			require.NoError(t, err)

			var serverInfo api.ServerUntrusted
			err = apiResponse.MetadataAsStruct(&serverInfo)
			require.NoError(t, err)

			require.Equal(t, tc.wantAuth, serverInfo.Auth)
		})
	}
}

func setupMockOIDC(t *testing.T) (*mockoidc.MockOIDC, map[string]string) {
	t.Helper()

	m, err := mockoidc.Run()
	require.NoError(t, err)

	t.Cleanup(func() {
		err = m.Shutdown()
		require.NoError(t, err)
	})

	accessTokens := make(map[string]string, len(users))

	for _, user := range users {
		m.QueueCode(oidcCode)
		sess, err := m.SessionStore.NewSession("openid email profile groups", "", &mockoidc.MockUser{
			Subject: user,
		}, oidcCode, "")
		require.NoError(t, err)

		accessToken, err := sess.AccessToken(&mockoidc.Config{
			ClientID:     m.Config().ClientID,
			ClientSecret: m.Config().ClientSecret,
			Issuer:       m.Issuer(),
			AccessTTL:    1 * time.Minute,
			RefreshTTL:   1 * time.Minute,
		}, m.Keypair, time.Now())
		require.NoError(t, err)

		accessTokens[user] = accessToken
	}

	return m, accessTokens
}

func setupOpenFGA(t *testing.T) (endpoint string, storeID string) {
	t.Helper()

	openFGAEndpoint := testingopenfga.Run(t)

	return openFGAEndpoint, createOpenFGAStore(t, openFGAEndpoint)
}

func createOpenFGAStore(t *testing.T, openFGAEndpoint string) string {
	t.Helper()

	resp, err := http.Post(openFGAEndpoint+"/stores", "application/json", bytes.NewBufferString(`{"name": "operations-center"}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	// The in-process HTTP API returns 200. A standalone OpenFGA server returns 201.
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create the OpenFGA store")

	createStoreResponseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var createStoreResponse struct {
		ID string `json:"id"`
	}

	err = json.Unmarshal(createStoreResponseBody, &createStoreResponse)
	require.NoError(t, err)
	require.NotEmpty(t, createStoreResponse.ID, "OpenFGA returned no store ID")

	return createStoreResponse.ID
}

func setupOpenFGATuples(t *testing.T, endpoint string, storeID string) {
	t.Helper()

	for _, user := range users {
		resp, err := http.Post(
			fmt.Sprintf("%s/stores/%s/write", endpoint, storeID),
			"application/json",
			bytes.NewBufferString(
				fmt.Sprintf(`{
  "writes": {
    "tuple_keys": [
      {
        "user": "user:%[1]s",
        "relation": "%[1]s",
        "object": "server:operations-center"
      }
    ]
  }
}`, user),
			),
		)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
}

func setupTokenAndSeeds(t *testing.T, varDir string) string {
	t.Helper()

	ocClient, err := client.New("http://unix.socket/", client.WithForceLocal(filepath.Join(varDir, "unix.socket")))
	require.NoError(t, err)

	err = ocClient.CreateToken(t.Context(), api.TokenPut{
		UsesRemaining: 10,
		ExpireAt:      time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	tokens, err := ocClient.GetTokens(t.Context())
	require.NoError(t, err)
	require.NotZero(t, tokens)

	tokenID := tokens[0].UUID.String()

	err = ocClient.CreateTokenSeed(t.Context(), tokenID, api.TokenSeedPost{
		Name: "public",
		TokenSeedPut: api.TokenSeedPut{
			Description: "public",
			Public:      true,
			Seeds:       api.TokenSeedConfigs{},
		},
	})
	require.NoError(t, err)

	err = ocClient.CreateTokenSeed(t.Context(), tokenID, api.TokenSeedPost{
		Name: "private",
		TokenSeedPut: api.TokenSeedPut{
			Description: "private",
			Public:      false,
			Seeds:       api.TokenSeedConfigs{},
		},
	})
	require.NoError(t, err)

	return tokenID
}

func readBody(t *testing.T, r io.Reader) string {
	t.Helper()

	body, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(body)
}
