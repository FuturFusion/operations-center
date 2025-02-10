package api_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/api"
	"github.com/FuturFusion/operations-center/cmd/operations-centerd/internal/config"
)

func TestStartAndStop(t *testing.T) {
	tmpDir := t.TempDir()

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

				resp, err := http.Get("http://localhost:17443/")
				require.NoError(t, err)
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				wantBody := `{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":["/1.0"]}`
				require.JSONEq(t, wantBody, string(body))
			},
		},
		{
			name:       "success - http request using subrouter",
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			bindPort:   17443,

			assertStartErr: require.NoError,
			assertStopErr:  require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				resp, err := http.Get("http://localhost:17443/1.0/provisioning/tokens")
				require.NoError(t, err)
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				wantBody := `{"type":"sync","status":"Success","status_code":200,"operation":"","error_code":0,"error":"","metadata":[]}`
				require.JSONEq(t, wantBody, string(body))
			},
		},
		{
			name:       "success - http request using subrouter with trailing slash - not found",
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			bindPort:   17443,

			assertStartErr: require.NoError,
			assertStopErr:  require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				resp, err := http.Get("http://localhost:17443/1.0/provisioning/tokens/")
				require.NoError(t, err)
				defer resp.Body.Close()

				require.Equal(t, http.StatusNotFound, resp.StatusCode)

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				wantBody := `{"type":"error","status":"","status_code":0,"operation":"","error_code":404,"error":"Not Found","metadata":null}`
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
			d := api.NewDaemon(
				mockEnv{
					unixSocket: tc.unixSocket,
					varDir:     tmpDir,
				},
				&config.Config{
					RestServerPort: tc.bindPort,
				},
			)

			err := d.Start()
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
