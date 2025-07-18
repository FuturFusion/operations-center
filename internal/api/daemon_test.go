package api_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
)

func TestStartAndStop(t *testing.T) {
	tmpDir := t.TempDir()

	// Add dummy server.crt.
	f, err := os.Create(filepath.Join(tmpDir, "server.crt"))
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	// Block port 17444
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf(":%d", 17444), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	}()

	tests := []struct {
		name       string
		unixSocket string
		bindPort   int

		assertStartErr require.ErrorAssertionFunc
		assertStopErr  require.ErrorAssertionFunc
		assertFunc     func(t *testing.T)
	}{
		{
			name:       "success - unix socket request",
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			bindPort:   17443,

			assertStartErr: require.NoError,
			assertStopErr:  require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				socketClient := &http.Client{
					Transport: &http.Transport{
						DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
							return net.Dial("unix", filepath.Join(tmpDir, "unix.socket"))
						},
					},
				}

				resp, err := socketClient.Get("http://unix")
				require.NoError(t, err)
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				wantBody := `{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":["/1.0"]}`
				require.JSONEq(t, wantBody, string(body))
			},
		},
		{
			name:       "success - http request",
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			bindPort:   17443,

			assertStartErr: require.NoError,
			assertStopErr:  require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				client := http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				}

				resp, err := client.Get("https://localhost:17443")
				require.NoError(t, err)
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				wantBody := `{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":["/1.0"]}`
				require.JSONEq(t, wantBody, string(body))
			},
		},
		{
			name:       "error - invalid unix socket",
			unixSocket: tmpDir, // invalid, because it is a directory
			bindPort:   17443,

			assertStartErr: require.Error,
			assertStopErr:  require.Error,
			assertFunc:     func(*testing.T) {},
		},
		{
			name:       "error - http port already taken",
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			bindPort:   17444,

			assertStartErr: require.Error,
			assertStopErr:  require.Error,
			assertFunc:     func(*testing.T) {},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			d := api.NewDaemon(
				ctx,
				mockEnv{
					unixSocket: tc.unixSocket,
					varDir:     tmpDir,
				},
				&config.Config{
					RestServerPort: tc.bindPort,
				},
			)

			err := d.Start(ctx)
			tc.assertStartErr(t, err)
			t.Cleanup(func() {
				err = d.Stop(context.Background())
				tc.assertStopErr(t, err)
			})

			tc.assertFunc(t)
		})
	}
}

type mockEnv struct {
	logDir     string
	varDir     string
	unixSocket string
}

func (e mockEnv) LogDir() string        { return e.logDir }
func (e mockEnv) VarDir() string        { return e.varDir }
func (e mockEnv) GetUnixSocket() string { return e.unixSocket }
