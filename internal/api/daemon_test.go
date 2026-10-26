package api_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	restapi "github.com/FuturFusion/operations-center/internal/api"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestStartAndStop(t *testing.T) {
	tmpDir := t.TempDir()

	// Add dummy server.crt.
	f, err := os.Create(filepath.Join(tmpDir, "server.crt"))
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	freePort := testingnet.FreePort(t, "")

	// Block a port. Close the listener after the test to not leak the port.
	blockingListener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "Failed to block a port")
	t.Cleanup(func() {
		require.NoError(t, blockingListener.Close(), "Failed to close the blocking listener")
	})

	blockedPort := strconv.Itoa(blockingListener.Addr().(*net.TCPAddr).Port)

	tests := []struct {
		name       string
		unixSocket string
		bindPort   string

		assertStartErr require.ErrorAssertionFunc
		assertStopErr  require.ErrorAssertionFunc
		assertFunc     func(t *testing.T)
	}{
		{
			name:       "success - unix socket request",
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			bindPort:   freePort,

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
			bindPort:   freePort,

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

				resp, err := client.Get("https://localhost:" + freePort)
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
			bindPort:   freePort,

			assertStartErr: require.Error,
			// Start fails before any listener is served, so there is nothing left to fail during stop.
			assertStopErr: require.NoError,
			assertFunc:    func(*testing.T) {},
		},
		{
			name:       "error - http port already taken",
			unixSocket: filepath.Join(tmpDir, "unix.socket"),
			bindPort:   blockedPort,

			assertStartErr: require.Error,
			assertStopErr:  require.Error,
			assertFunc:     func(*testing.T) {},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			env := &mock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
				GetUnixSocketFunc: func() string {
					return tc.unixSocket
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
				ClusteredFunc: func() bool { return false },
			}

			config.InitTest(t, env, nil, config.InternalConfig{
				IsBackgroundTasksDisabled: true,
				SourcePollSkipFirst:       true,
			})
			err := config.UpdateLocalNetwork(ctx, system.NetworkPut{
				RestServerAddress: ":" + tc.bindPort,
			})
			require.NoError(t, err)

			err = config.UpdateNetwork(ctx, system.NetworkGlobalPut{
				OperationsCenterAddress: "https://127.0.0.1:" + freePort,
			})
			require.NoError(t, err)

			d := restapi.NewDaemon(ctx, env)

			err = d.Start(ctx)
			tc.assertStartErr(t, err)
			t.Cleanup(func() {
				err = d.Stop(context.Background())
				tc.assertStopErr(t, err)
			})

			tc.assertFunc(t)
		})
	}
}
