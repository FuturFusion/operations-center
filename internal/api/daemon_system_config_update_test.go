package api_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
)

func TestSystemConfigUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Authentication tests are slow due to the use of test containers")
	}

	err := os.Setenv("OPERATIONS_CENTER_DISABLE_BACKGROUND_TASKS", "true")
	require.NoError(t, err)

	// Setup
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

	// Setup OIDC mock service.
	oidcProvider, accessTokens := setupMockOIDC(t)

	// Start OpenFGA container.
	openFGAEndpoint, openFGAStoreID := setupOpenFGA(ctx, t)

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

	// Setup daemon with empty (default) configuration for actual tests.
	config.InitTest(t, env, nil, config.InternalConfig{
		IsBackgroundTasksDisabled: true,
		SourcePollSkipFirst:       true,
	})

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

	// Prepare clients for TCP and unix socket.
	tcpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	socketClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
			},
		},
	}

	port := 17443
	openFGAInitialized := false
	for i := range 4 {
		t.Log(`1. Expect error over http with no rest port defined`)
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/1.0", port), http.NoBody)
		require.NoError(t, err)
		resp, err := tcpClient.Do(req)
		require.Error(t, err)
		if resp != nil {
			// Make linter happy.
			_ = resp.Body.Close()
		}

		t.Log(`2. Update network configuration, listen on port 17443`)
		req, err = http.NewRequest(http.MethodPut, "http://unix.socket/1.0/system/network", bytes.NewBufferString(fmt.Sprintf(`{
  "address": "https://127.0.0.1:17443",
  "rest_server_address": "[::1]:%d"
}
`, port)))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/json")
		resp, err = socketClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Expect unauthorized over http without trusted credentials.
		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/1.0", port), http.NoBody)
		require.NoError(t, err)
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		t.Log(`3. Update trusted TLS client certificates`)
		req, err = http.NewRequest(http.MethodPut, "http://unix.socket/1.0/system/security", bytes.NewBufferString(`{
  "trusted_tls_client_cert_fingerprints": [ "`+certFingerprint+`" ]
}
`))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/json")
		resp, err = socketClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Expect successful call with client certificate.
		tcpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates:       []tls.Certificate{cert},
					InsecureSkipVerify: true,
				},
			},
		}

		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/1.0", port), http.NoBody)
		require.NoError(t, err)
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		t.Log(`4. Expect unauthorized with user "admin" without OIDC configuration`)
		tcpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}

		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/1.0", port), http.NoBody)
		require.NoError(t, err)
		req.Header.Add("Authorization", "Bearer "+accessTokens[admin])
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		t.Log(`5. Add OIDC configuration`)
		req, err = http.NewRequest(http.MethodPut, "http://unix.socket/1.0/system/security", bytes.NewBufferString(`{
  "oidc": {
    "issuer": "`+oidcProvider.Issuer()+`",
    "client_id": "`+oidcProvider.ClientID+`",
    "scope": "openid,offline_access,email"
  }
}
`))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/json")
		resp, err = socketClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Expect successful call with user "admin" and OIDC configuration set.
		tcpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}

		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/1.0", port), http.NoBody)
		require.NoError(t, err)
		req.Header.Add("Authorization", "Bearer "+accessTokens[admin])
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		t.Log(`6. Add OpenFGA configuration`)
		// Add it using user "viewer", which works now, since no authorization is happening.
		req, err = http.NewRequest(http.MethodPut, fmt.Sprintf("https://localhost:%d/1.0/system/security", port), bytes.NewBufferString(`{
  "oidc": {
    "issuer": "`+oidcProvider.Issuer()+`",
    "client_id": "`+oidcProvider.ClientID+`",
    "scope": "openid,offline_access,email"
  },
  "openfga": {
    "api_token": "dummy",
    "api_url": "`+openFGAEndpoint+`",
    "store_id": "`+openFGAStoreID+`"
  },
  "trusted_tls_client_cert_fingerprints": [ "dummy" ]
}
`))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", "Bearer "+accessTokens[viewer])
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()

		// With OpenFGA now configured, we need to add the permission tuples to the
		// store on the first run.
		if !openFGAInitialized {
			setupOpenFGATuples(t, openFGAEndpoint, openFGAStoreID)
			openFGAInitialized = true
		}

		// Expect successful call with viewer if OIDC and OpenFGA configuration set.
		tcpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}

		req, err = http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/1.0", port), http.NoBody)
		require.NoError(t, err)
		req.Header.Add("Authorization", "Bearer "+accessTokens[viewer])
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Expect PUT to fail now, since user "viewer" is not allowed to update security settings.
		req, err = http.NewRequest(http.MethodPut, fmt.Sprintf("https://localhost:%d/1.0/system/security", port), bytes.NewBufferString(`{
  "oidc": {
    "issuer": "`+oidcProvider.Issuer()+`",
    "client_id": "`+oidcProvider.ClientID+`",
    "scope": "openid,offline_access,email"
  },
  "openfga": {
    "api_token": "dummy",
    "api_url": "`+openFGAEndpoint+`",
    "store_id": "`+openFGAStoreID+`"
  }
}
`))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", "Bearer "+accessTokens[viewer])
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusForbidden, resp.StatusCode)

		t.Log(`7. Reset security config with user "admin`)
		req, err = http.NewRequest(http.MethodPut, fmt.Sprintf("https://localhost:%d/1.0/system/security", port), bytes.NewBufferString(`{}`))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", "Bearer "+accessTokens[admin])
		resp, err = tcpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		t.Log(`8. Reset network config over unix socket`)
		req, err = http.NewRequest(http.MethodPut, "http://unix.socket/1.0/system/network", bytes.NewBufferString(`{}`))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/json")
		resp, err = socketClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Switch between port 17443 and 17444 back and forth.
		if i%2 == 0 {
			port = 17444
		} else {
			port = 17443
		}
	}
}
