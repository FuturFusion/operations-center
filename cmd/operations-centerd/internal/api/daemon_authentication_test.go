package api_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/api"
	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/config"
)

func TestAuthentication(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		client   func() *http.Client
		resource string

		wantStatusCode int
	}{
		{
			name: "plain http /",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			resource: "https://localhost:17443/",

			wantStatusCode: http.StatusOK,
		},
		{
			name: "socket /",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
						},
					},
				}
			},
			resource: "http://unix.socket/",

			wantStatusCode: http.StatusOK,
		},
		{
			name: "plain http /1.0 - forbidden",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}
			},
			resource: "https://localhost:17443/1.0",

			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "socket /1.0",
			client: func() *http.Client {
				return &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
						},
					},
				}
			},
			resource: "http://unix.socket/1.0",

			wantStatusCode: http.StatusOK,
		},
	}

	ctx := context.Background()
	d := api.NewDaemon(
		ctx,
		mockEnv{
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			varDir:     tmpDir,
		},
		&config.Config{
			RestServerPort: 17443,
		},
	)

	err := d.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

	for _, tc := range tests {
		t.Run(tc.resource, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.resource, http.NoBody)
			require.NoError(t, err)

			resp, err := tc.client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, tc.wantStatusCode, resp.StatusCode)
		})
	}
}
