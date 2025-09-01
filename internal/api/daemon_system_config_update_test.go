package api_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestSystemConfigUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Authentication tests are slow due to the use of test containers")
	}

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

	oidcProvider, accessTokens := setupMockOIDC(t)

	openFGAEndpoint, openFGAStoreID := setupOpenFGA(ctx, t)

	config.InitTest(t)
	err = config.UpdateNetwork(ctx, api.SystemNetworkPut{
		OperationsCenterAddress: "https://127.0.0.1:17443",
		RestServerPort:          17443,
	})
	require.NoError(t, err)
	err = config.UpdateSecurity(ctx, api.SystemSecurityPut{
		TrustedTLSClientCertFingerprints: []string{certFingerprint},
		OpenFGA: api.SystemSecurityOpenFGA{
			APIURL:   openFGAEndpoint,
			APIToken: "dummy",
			StoreID:  openFGAStoreID,
		},
	})
	require.NoError(t, err)

	d := restapi.NewDaemon(
		ctx,
		restapi.MockEnv{
			UnixSocket:   filepath.Join(tmpDir, "unix.socket"),
			VarDirectory: tmpDir,
		},
	)

	err = d.Start(ctx)
	require.NoError(t, err)
	err = d.Stop(context.Background())
	require.NoError(t, err)
	setupOpenFGATuples(t, openFGAEndpoint, openFGAStoreID)

	config.InitTest(t)
	err = config.UpdateNetwork(ctx, api.SystemNetworkPut{
		OperationsCenterAddress: "https://127.0.0.1:17443",
		RestServerPort:          17443,
	})
	require.NoError(t, err)

	d = restapi.NewDaemon(
		ctx,
		restapi.MockEnv{
			UnixSocket:   filepath.Join(tmpDir, "unix.socket"),
			VarDirectory: tmpDir,
		},
	)
	err = d.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	unixSocketClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
			},
		},
	}

	// 1. Expect forbidden over http without trusted credentials.
	req, err := http.NewRequest(http.MethodGet, "https://localhost:17443/1.0", http.NoBody)
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// 2. Update network configuration, listen on port 17444.
	req, err = http.NewRequest(http.MethodPut, "http://unix.socket/1.0/system/network", bytes.NewBufferString(`{
  "address": "https://127.0.0.1:17443",
  "rest_server_address": "127.0.0.1",
  "rest_server_port": 17444
}
`))
	req.Header.Add("Content-Type", "application/json")
	resp, err = unixSocketClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Still expect forbidden, but now on port 17444.
	req, err = http.NewRequest(http.MethodGet, "https://localhost:17444/1.0", http.NoBody)
	resp, err = httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// 3. Update trusted TLS client certificates.
	req, err = http.NewRequest(http.MethodPut, "http://unix.socket/1.0/system/security", bytes.NewBufferString(`{
  "trusted_tls_client_cert_fingerprints": [ "`+certFingerprint+`" ]
}
`))
	req.Header.Add("Content-Type", "application/json")
	resp, err = unixSocketClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Expect successful call with client certificate.
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates:       []tls.Certificate{cert},
				InsecureSkipVerify: true,
			},
		},
	}

	req, err = http.NewRequest(http.MethodGet, "https://localhost:17444/1.0", http.NoBody)
	resp, err = httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 4. Expect forbidden with admin without OIDC configuration.
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err = http.NewRequest(http.MethodGet, "https://localhost:17444/1.0", http.NoBody)
	req.Header.Add("Authorization", "Bearer "+accessTokens[admin])
	resp, err = httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// 5. Add OIDC configuration.
	req, err = http.NewRequest(http.MethodPut, "http://unix.socket/1.0/system/security", bytes.NewBufferString(`{
  "oidc": {
    "issuer": "`+oidcProvider.Issuer()+`",
    "client_id": "`+oidcProvider.ClientID+`",
    "scope": "openid,offline_access,email"
  }
}
`))
	req.Header.Add("Content-Type", "application/json")
	resp, err = unixSocketClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Expect successful call with admin and OIDC configuration set.
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err = http.NewRequest(http.MethodGet, "https://localhost:17444/1.0", http.NoBody)
	req.Header.Add("Authorization", "Bearer "+accessTokens[admin])
	resp, err = httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 6. Add OpenFGA configuration.
	// Add it using viewer, which works now, since no authorization is happening.
	req, err = http.NewRequest(http.MethodPut, "https://localhost:17444/1.0/system/security", bytes.NewBufferString(`{
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
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+accessTokens[viewer])
	resp, err = httpClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Expect successful call with viewer if OIDC and OpenFGA configuration set.
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err = http.NewRequest(http.MethodGet, "https://localhost:17444/1.0", http.NoBody)
	req.Header.Add("Authorization", "Bearer "+accessTokens[viewer])
	resp, err = httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Expect PUT to fail now, since viewer is not allowed to update security settings.
	req, err = http.NewRequest(http.MethodPut, "https://localhost:17444/1.0/system/security", bytes.NewBufferString(`{
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
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+accessTokens[viewer])
	resp, err = httpClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}
