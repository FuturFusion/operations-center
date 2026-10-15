package api_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
	testingopenfga "github.com/FuturFusion/operations-center/internal/util/testing/openfga"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestOpenFGAOutage(t *testing.T) {
	if testing.Short() {
		t.Skip("OpenFGA outage test is slow")
	}

	// Setup
	ctx := context.Background()
	tmpDir := t.TempDir()

	f, err := os.Create(filepath.Join(tmpDir, "server.crt"))
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	oidcProvider, accessTokens := setupMockOIDC(t)

	openFGAEndpoint, stopOpenFGA := testingopenfga.RunStoppable(t)
	openFGAStoreID := createOpenFGAStore(t, openFGAEndpoint)

	port := testingnet.FreePort(t, "")
	tcpBaseURL := "https://localhost:" + port

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
	err = config.UpdateSecurity(ctx, system.SecurityPut{
		TrustedTLSClientCertFingerprints: []string{incustls.CertFingerprint(cert.Leaf)},
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

	d := restapi.NewDaemon(ctx, env)

	err = d.Start(ctx)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

	require.NoError(t, err)

	setupOpenFGATuples(t, openFGAEndpoint, openFGAStoreID)

	oidcClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	certClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates:       []tls.Certificate{cert},
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

	statusCode := func(t *testing.T, client *http.Client, resource string, headers map[string]string) int {
		t.Helper()

		req, err := http.NewRequest(http.MethodGet, resource, http.NoBody)
		require.NoError(t, err)

		for key, value := range headers {
			req.Header.Add(key, value)
		}

		resp, err := client.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		return resp.StatusCode
	}

	tests := []struct {
		name     string
		client   *http.Client
		resource string
		headers  map[string]string

		wantStatusCodeBefore int
		wantStatusCodeDuring int
	}{
		{
			name:     "oidc GET /1.0/provisioning/tokens as admin",
			client:   oidcClient,
			resource: tcpBaseURL + "/1.0/provisioning/tokens",
			headers:  map[string]string{"Authorization": "Bearer " + accessTokens[admin]},

			wantStatusCodeBefore: http.StatusOK,
			wantStatusCodeDuring: http.StatusForbidden,
		},
		{
			name:     "oidc GET /1.0/provisioning/tokens as viewer",
			client:   oidcClient,
			resource: tcpBaseURL + "/1.0/provisioning/tokens",
			headers:  map[string]string{"Authorization": "Bearer " + accessTokens[viewer]},

			wantStatusCodeBefore: http.StatusOK,
			wantStatusCodeDuring: http.StatusForbidden,
		},
		{
			name:     "oidc GET /1.0/system/security as admin",
			client:   oidcClient,
			resource: tcpBaseURL + "/1.0/system/security",
			headers:  map[string]string{"Authorization": "Bearer " + accessTokens[admin]},

			wantStatusCodeBefore: http.StatusOK,
			wantStatusCodeDuring: http.StatusForbidden,
		},
		{
			name:     "client cert GET /1.0/provisioning/tokens",
			client:   certClient,
			resource: tcpBaseURL + "/1.0/provisioning/tokens",

			wantStatusCodeBefore: http.StatusOK,
			wantStatusCodeDuring: http.StatusOK,
		},
		{
			name:     "client cert GET /1.0/system/security",
			client:   certClient,
			resource: tcpBaseURL + "/1.0/system/security",

			wantStatusCodeBefore: http.StatusOK,
			wantStatusCodeDuring: http.StatusOK,
		},
		{
			name:     "unix socket GET /1.0/provisioning/tokens",
			client:   socketClient,
			resource: "http://unix.socket/1.0/provisioning/tokens",

			wantStatusCodeBefore: http.StatusOK,
			wantStatusCodeDuring: http.StatusOK,
		},
		{
			name:     "unix socket GET /1.0/system/security",
			client:   socketClient,
			resource: "http://unix.socket/1.0/system/security",

			wantStatusCodeBefore: http.StatusOK,
			wantStatusCodeDuring: http.StatusOK,
		},
	}

	for _, tc := range tests {
		require.Equal(t, tc.wantStatusCodeBefore, statusCode(t, tc.client, tc.resource, tc.headers), "unexpected status code before the OpenFGA outage for %q", tc.name)
	}

	// Run test
	stopOpenFGA()

	// Assert
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantStatusCodeDuring, statusCode(t, tc.client, tc.resource, tc.headers), "unexpected status code during the OpenFGA outage")
		})
	}
}
