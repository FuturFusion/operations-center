package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
	viewer   = "viewer"
)

var users = []string{
	admin,
	operator,
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

	metricsCertPEM, metricsKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err, "generating the metrics only certificate must work")

	metricsCert, err := tls.X509KeyPair(metricsCertPEM, metricsKeyPEM)
	require.NoError(t, err, "loading the metrics only certificate must work")

	// Use the format of openssl, upper case with colons, to cover the
	// canonicalization of fingerprints from the configuration.
	metricsCertFingerprint := strings.ReplaceAll(fmt.Sprintf("% X", sha256.Sum256(metricsCert.Leaf.Raw)), " ", ":")

	metricsPEMCertPEM, metricsPEMKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err, "generating the metrics only certificate, trusted by its PEM encoded certificate, must work")

	metricsPEMCert, err := tls.X509KeyPair(metricsPEMCertPEM, metricsPEMKeyPEM)
	require.NoError(t, err, "loading the metrics only certificate, trusted by its PEM encoded certificate, must work")

	tlsClient := func(cert tls.Certificate) func() *http.Client {
		return func() *http.Client {
			return &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						Certificates:       []tls.Certificate{cert},
						InsecureSkipVerify: true,
					},
				},
			}
		}
	}

	oidcProvider, accessTokens := setupMockOIDC(t)

	openFGAEndpoint, openFGAStoreID := setupOpenFGA(t)

	var token string

	port := testingnet.FreePort(t, "")
	tcpBaseURL := "https://localhost:" + port

	// Test cases
	tests := []struct {
		name     string
		client   func() *http.Client
		method   string
		resource string
		headers  map[string]string
		body     io.Reader

		wantStatusCode   int
		wantBodyContains string
		wantAuth         string // Only used in tests for /1.0 route.
	}{
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
			name:     "metrics client cert http GET /1.0",
			client:   tlsClient(metricsCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
			wantAuth:       api.AuthenticationMethodTLS,
		},
		{
			name:     "metrics client cert by PEM http GET /1.0",
			client:   tlsClient(metricsPEMCert),
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
			name:     "metrics client cert http GET /1.0/metrics",
			client:   tlsClient(metricsCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/metrics",
			body:     http.NoBody,

			wantStatusCode:   http.StatusOK,
			wantBodyContains: "operations_center_build_info",
		},
		{
			name:     "metrics client cert http GET /1.0/system/settings - forbidden",
			client:   tlsClient(metricsCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/system/settings",
			body:     http.NoBody,

			wantStatusCode:   http.StatusForbidden,
			wantBodyContains: `entitlement \"can_view\"`,
		},
		{
			name:     "metrics client cert http GET /1.0/provisioning/servers - forbidden",
			client:   tlsClient(metricsCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			body:     http.NoBody,

			wantStatusCode:   http.StatusForbidden,
			wantBodyContains: `entitlement \"can_view\"`,
		},
		{
			name:     "metrics client cert http PUT /1.0/system/settings - forbidden",
			client:   tlsClient(metricsCert),
			method:   http.MethodPut,
			resource: tcpBaseURL + "/1.0/system/settings",
			body:     strings.NewReader(`{}`),

			wantStatusCode:   http.StatusForbidden,
			wantBodyContains: `entitlement \"can_edit\"`,
		},
		{
			name:     "metrics client cert http POST /1.0/provisioning/servers - forbidden",
			client:   tlsClient(metricsCert),
			method:   http.MethodPost,
			resource: tcpBaseURL + "/1.0/provisioning/servers",
			body:     strings.NewReader(`{}`),

			wantStatusCode:   http.StatusForbidden,
			wantBodyContains: `entitlement \"can_create\"`,
		},
		{
			name:     "metrics client cert http GET /1.0/debug/pprof/ - forbidden",
			client:   tlsClient(metricsCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/debug/pprof/",
			body:     http.NoBody,

			wantStatusCode:   http.StatusForbidden,
			wantBodyContains: `entitlement \"can_debug\"`,
		},
		{
			name:     "metrics client cert http GET /os/1.0 - forbidden",
			client:   tlsClient(metricsCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/os/1.0",
			body:     http.NoBody,

			wantStatusCode:   http.StatusForbidden,
			wantBodyContains: `entitlement \"can_edit\"`,
		},
		{
			name:     "metrics client cert by PEM http GET /1.0/metrics",
			client:   tlsClient(metricsPEMCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/metrics",
			body:     http.NoBody,

			wantStatusCode:   http.StatusOK,
			wantBodyContains: "operations_center_build_info",
		},
		{
			name:     "metrics client cert by PEM http GET /1.0/system/settings - forbidden",
			client:   tlsClient(metricsPEMCert),
			method:   http.MethodGet,
			resource: tcpBaseURL + "/1.0/system/settings",
			body:     http.NoBody,

			wantStatusCode:   http.StatusForbidden,
			wantBodyContains: `entitlement \"can_view\"`,
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
	}

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
	}

	config.InitTest(t, env, nil, config.InternalConfig{
		IsBackgroundTasksDisabled: true,
		SourcePollSkipFirst:       true,
	})
	err = config.UpdateNetwork(ctx, system.NetworkPut{
		OperationsCenterAddress: "https://127.0.0.1:" + port,
		RestServerAddress:       testingnet.LocalhostIP(t) + ":" + port,
	})
	require.NoError(t, err)
	security := system.SecurityPut{
		TrustedTLSClientCertFingerprints:        []string{certFingerprint},
		TrustedTLSMetricsClientCertFingerprints: []string{metricsCertFingerprint},
		TrustedTLSMetricsClientCertificates:     []string{string(metricsPEMCertPEM)},
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
	}

	err = config.UpdateSecurity(ctx, security)
	require.NoError(t, err, "the security config with metrics only certificates must be accepted")

	// The daemon command applies the metrics settings at startup.
	metrics.Apply(config.GetSettings().MetricsEnabled, config.GetSettings().MetricsComponents)
	t.Cleanup(func() { metrics.Apply(false, nil) })

	d := restapi.NewDaemon(
		ctx,
		env,
	)

	err = d.Start(ctx)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

	require.NoError(t, err)

	setupOpenFGATuples(t, openFGAEndpoint, openFGAStoreID)

	// An OpenFGA tuple must not widen the access of a metrics only certificate.
	writeOpenFGATuple(t, openFGAEndpoint, openFGAStoreID, incustls.CertFingerprint(metricsCert.Leaf), admin)
	writeOpenFGATuple(t, openFGAEndpoint, openFGAStoreID, incustls.CertFingerprint(metricsPEMCert.Leaf), admin)

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

	type reloadRequest struct {
		name     string
		client   func() *http.Client
		resource string

		wantStatusCode int
	}

	// The reloads change the security config, so these run after all other cases
	// and in the given order. Each mutation builds on the previous one.
	reloads := []struct {
		name     string
		mutate   func(security *system.SecurityPut)
		requests []reloadRequest
	}{
		{
			name: "remove metrics only certificates",
			mutate: func(security *system.SecurityPut) {
				security.TrustedTLSMetricsClientCertFingerprints = nil
				security.TrustedTLSMetricsClientCertificates = nil
			},
			requests: []reloadRequest{
				{
					name:     "metrics client cert http GET /1.0/metrics - revoked",
					client:   tlsClient(metricsCert),
					resource: tcpBaseURL + "/1.0/metrics",

					wantStatusCode: http.StatusUnauthorized,
				},
				{
					name:     "metrics client cert by PEM http GET /1.0/metrics - revoked",
					client:   tlsClient(metricsPEMCert),
					resource: tcpBaseURL + "/1.0/metrics",

					wantStatusCode: http.StatusUnauthorized,
				},
				{
					name:     "tls client cert http GET /1.0/metrics - still trusted",
					client:   tlsClient(cert),
					resource: tcpBaseURL + "/1.0/metrics",

					wantStatusCode: http.StatusOK,
				},
			},
		},
		{
			name: "add metrics only certificate",
			mutate: func(security *system.SecurityPut) {
				security.TrustedTLSMetricsClientCertFingerprints = []string{metricsCertFingerprint}
			},
			requests: []reloadRequest{
				{
					name:     "metrics client cert http GET /1.0/metrics - granted",
					client:   tlsClient(metricsCert),
					resource: tcpBaseURL + "/1.0/metrics",

					wantStatusCode: http.StatusOK,
				},
				{
					name:     "metrics client cert http GET /1.0/system/settings - forbidden",
					client:   tlsClient(metricsCert),
					resource: tcpBaseURL + "/1.0/system/settings",

					wantStatusCode: http.StatusForbidden,
				},
				{
					name:     "tls client cert http GET /1.0/system/settings - still trusted",
					client:   tlsClient(cert),
					resource: tcpBaseURL + "/1.0/system/settings",

					wantStatusCode: http.StatusOK,
				},
			},
		},
	}

	for _, reload := range reloads {
		t.Run(reload.name, func(t *testing.T) {
			reload.mutate(&security)

			err := config.UpdateSecurity(ctx, security)
			require.NoError(t, err, "the changed security config must be accepted")

			for _, tc := range reload.requests {
				t.Run(tc.name, func(t *testing.T) {
					// Setup
					req, err := http.NewRequest(http.MethodGet, tc.resource, http.NoBody)
					require.NoError(t, err, "building the request must work")

					// Run test
					resp, err := tc.client().Do(req)

					// Assert
					require.NoError(t, err, "the request must get a response")
					t.Cleanup(func() { _ = resp.Body.Close() })

					require.Equal(t, tc.wantStatusCode, resp.StatusCode, "a change of the metrics only certificates must take effect without a restart: %s", readBody(t, resp.Body))
				})
			}
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

	return openFGAEndpoint, createStoreResponse.ID
}

func setupOpenFGATuples(t *testing.T, endpoint string, storeID string) {
	t.Helper()

	for _, user := range users {
		writeOpenFGATuple(t, endpoint, storeID, user, user)
	}
}

func writeOpenFGATuple(t *testing.T, endpoint string, storeID string, user string, relation string) {
	t.Helper()

	resp, err := http.Post(
		fmt.Sprintf("%s/stores/%s/write", endpoint, storeID),
		"application/json",
		bytes.NewBufferString(
			fmt.Sprintf(`{
  "writes": {
    "tuple_keys": [
      {
        "user": "user:%s",
        "relation": "%s",
        "object": "server:operations-center"
      }
    ]
  }
}`, user, relation),
		),
	)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
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
