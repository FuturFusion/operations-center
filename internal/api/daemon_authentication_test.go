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
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/oauth2-proxy/mockoidc"
	"github.com/stretchr/testify/require"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/openfga"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	"github.com/FuturFusion/operations-center/internal/client"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/shared/api"
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
	if testing.Short() {
		t.Skip("Authentication tests are slow due to the use of test containers")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	err := os.Setenv("OPERATIONS_CENTER_DISABLE_BACKGROUND_TASKS", "true")
	require.NoError(t, err)

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
			name: "plain http GET /1.0 - unauthorized",
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

			wantStatusCode: http.StatusUnauthorized,
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

		// GET /1.0/provisioning/tokens/{token}/seeds/public does not need
		// authentication, since this seed is created with the public flag set to
		// true during setup.
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
			resource: "https://localhost:17443/1.0/provisioning/tokens/TOKEN/seeds/public",
			body:     http.NoBody,

			wantStatusCode: http.StatusOK,
		},
		// GET /1.0/provisioning/tokens/{token}/seeds/privat does need
		// authentication, since this seed is created with the public flag set to
		// false during setup.
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
			resource: "https://localhost:17443/1.0/provisioning/tokens/TOKEN/seeds/private",
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
		UsrShareDirFunc: func() string {
			return tmpDir
		},
	}

	config.InitTest(t, env)
	err = config.UpdateNetwork(ctx, api.SystemNetworkPut{
		OperationsCenterAddress: "https://127.0.0.1:17443",
		RestServerAddress:       "[::1]:17443",
	})
	require.NoError(t, err)
	err = config.UpdateSecurity(ctx, api.SystemSecurityPut{
		TrustedTLSClientCertFingerprints: []string{certFingerprint},
		OIDC: api.SystemSecurityOIDC{
			Issuer:   oidcProvider.Issuer(),
			ClientID: oidcProvider.ClientID,
			Scope:    "openid,offline_access,email",
		},
		OpenFGA: api.SystemSecurityOpenFGA{
			APIURL:   openFGAEndpoint,
			APIToken: "dummy",
			StoreID:  openFGAStoreID,
		},
	})
	require.NoError(t, err)

	d := restapi.NewDaemon(
		ctx,
		env,
	)

	err = d.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

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
