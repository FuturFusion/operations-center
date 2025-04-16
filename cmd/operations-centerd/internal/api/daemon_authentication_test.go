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
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/openfga"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/api"
	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/config"
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

	// Setup client certificate
	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	certFingerprint := incustls.CertFingerprint(cert.Leaf)

	oidcProvider, accessTokens := setupMockOIDC(t)

	openFGAEndpoint, openFGAStoreID := setupOpenFGA(ctx, t)

	// Test cases
	tests := []struct {
		name     string
		client   func() *http.Client
		method   string
		resource string
		headers  map[string]string
		body     io.Reader

		wantStatusCode int
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
			resource: "https://localhost:17443/",
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
			name: "plain http GET /1.0 - forbidden",
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
			resource: "https://localhost:17443/1.0",
			body:     http.NoBody,

			wantStatusCode: http.StatusForbidden,
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
			resource: "https://localhost:17443/1.0",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http GET /1.0 as viewer",
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
			resource: "https://localhost:17443/1.0",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http GET /1.0/provisioning/servers as viewer",
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
			resource: "https://localhost:17443/1.0/provisioning/servers",
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
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: "https://localhost:17443/1.0/provisioning/servers",
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
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodGet,
			resource: "https://localhost:17443/1.0/provisioning/servers",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		{
			name: "oidc http POST /1.0/provisioning/servers as viewer - forbidden",
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
			resource: "https://localhost:17443/1.0/provisioning/servers",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http POST /1.0/provisioning/servers as operator - forbidden",
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
			resource: "https://localhost:17443/1.0/provisioning/servers",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http POST /1.0/provisioning/servers as admin",
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
			resource: "https://localhost:17443/1.0/provisioning/servers",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusCreated,
		},
		{
			name: "oidc http PUT /1.0/provisioning/servers/serverA as viewer",
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
			resource: "https://localhost:17443/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[viewer],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://viewer:12346/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "oidc http PUT /1.0/provisioning/servers/serverA as operator",
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
			resource: "https://localhost:17443/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[operator],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://operator:12346/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusCreated,
		},
		{
			name: "oidc http PUT /1.0/provisioning/servers/serverA as admin",
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
			resource: "https://localhost:17443/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://admin:12346/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusCreated,
		},
		{
			name: "oidc http DELETE /1.0/provisioning/servers/serverA as viewer",
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
			method:   http.MethodDelete,
			resource: "https://localhost:17443/1.0/provisioning/servers/serverA",
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
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodDelete,
			resource: "https://localhost:17443/1.0/provisioning/servers/serverA",
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
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodDelete,
			resource: "https://localhost:17443/1.0/provisioning/servers/serverA",
			headers: map[string]string{
				"Authorization": "Bearer " + accessTokens[admin],
			},
			body: http.NoBody,

			wantStatusCode: http.StatusOK,
		},
	}

	d := api.NewDaemon(
		ctx,
		mockEnv{
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			varDir:     tmpDir,
		},
		&config.Config{
			RestServerPort:                   17443,
			OidcIssuer:                       oidcProvider.Issuer(),
			OidcClientID:                     oidcProvider.ClientID,
			OidcScope:                        "openid,offline_access,email",
			TrustedTLSClientCertFingerprints: []string{certFingerprint},
			OpenfgaAPIURL:                    openFGAEndpoint,
			OpenfgaAPIToken:                  "dummy",
			OpenfgaStoreID:                   openFGAStoreID,
		},
	)

	err = d.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

	setupOpenFGATuples(t, openFGAEndpoint, openFGAStoreID)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.resource, tc.body)
			require.NoError(t, err)

			for key, value := range tc.headers {
				req.Header.Add(key, value)
			}

			resp, err := tc.client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, tc.wantStatusCode, resp.StatusCode)
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

func setupOpenFGA(ctx context.Context, t *testing.T) (endpoint string, storeID string) {
	t.Helper()

	openfgaContainer, err := openfga.Run(ctx, "openfga/openfga:v1.8.9")
	require.NoError(t, err)

	t.Cleanup(func() {
		err = testcontainers.TerminateContainer(openfgaContainer)
		require.NoError(t, err)
	})

	openFGAEndpoint, err := openfgaContainer.PortEndpoint(ctx, nat.Port("8080"), "")
	require.NoError(t, err)
	openFGAEndpoint = "http://" + openFGAEndpoint

	resp, err := http.Post(openFGAEndpoint+"/stores", "application/json", bytes.NewBufferString(`{"name": "operations-center"}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	createStoreResponseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var createStoreResponse struct {
		ID string `json:"id"`
	}

	err = json.Unmarshal(createStoreResponseBody, &createStoreResponse)
	require.NoError(t, err)

	return openFGAEndpoint, createStoreResponse.ID
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
