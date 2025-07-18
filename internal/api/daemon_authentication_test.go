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
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/openfga"

	"github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
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

	// Setup alternative client certificate
	altCertPEM, altKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	altCert, err := tls.X509KeyPair(altCertPEM, altKeyPEM)
	require.NoError(t, err)

	oidcProvider, accessTokens := setupMockOIDC(t)

	openFGAEndpoint, openFGAStoreID := setupOpenFGA(ctx, t)

	var token string

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
			name: "oidc http POST /1.0/provisioning/tokens as viewer - forbidden",
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
			resource: "https://localhost:17443/1.0/provisioning/tokens",
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
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: "https://localhost:17443/1.0/provisioning/tokens",
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
							Certificates:       []tls.Certificate{cert},
							InsecureSkipVerify: true,
						},
					},
				}
			},
			method:   http.MethodPost,
			resource: "https://localhost:17443/1.0/provisioning/tokens",
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
			resource: "https://localhost:17443/1.0/provisioning/servers?token=TOKEN",
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusCreated,
		},
		{
			name: "http POST /1.0/provisioning/servers without token",
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
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusBadRequest,
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
			resource: "https://localhost:17443/1.0/provisioning/servers?token=invalid",
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "server_type": "incus"
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
			resource: "https://localhost:17443/1.0/provisioning/servers?token=01a37f54-4dbd-4d26-a88d-df5a534545fb",
			body: bytes.NewBufferString(`{
  "name": "serverA",
  "connection_url": "https://127.0.0.1:12345/",
  "server_type": "incus"
}`),

			wantStatusCode: http.StatusForbidden,
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
  "server_type": "incus",
  "server_status": "ready"
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
  "server_type": "incus",
  "server_status": "ready"
}`),

			wantStatusCode: http.StatusOK,
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
  "server_type": "incus",
  "server_status": "ready"
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
			resource: "https://localhost:17443/1.0/provisioning/servers/:self",
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
			resource: "https://localhost:17443/1.0/provisioning/servers/:self",
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
			resource: "https://localhost:17443/1.0/provisioning/servers/:self",
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
			resource: "https://localhost:17443/1.0/provisioning/updates",
			body:     http.NoBody,

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

	token = setupToken(t, tmpDir)

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

func setupToken(t *testing.T, varDir string) string {
	t.Helper()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", filepath.Join(varDir, "unix.socket"))
			},
		},
	}

	req, err := http.NewRequest(http.MethodPost, "http://unix.socket/1.0/provisioning/tokens", bytes.NewBufferString(`{
  "uses_remaining": 10,
  "expire_at": "2099-12-31T23:59:59Z"
}`))
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	req, err = http.NewRequest(http.MethodGet, "http://unix.socket/1.0/provisioning/tokens?recursion=1", http.NoBody)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	token := struct {
		Metadata []struct {
			UUID string `json:"uuid"`
		} `json:"metadata"`
	}{}

	err = json.NewDecoder(resp.Body).Decode(&token)
	require.NoError(t, err)

	return token.Metadata[0].UUID
}
