package client_test

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/api"
	"github.com/FuturFusion/operations-center/internal/client"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	shared "github.com/FuturFusion/operations-center/shared/api"
)

func daemonSetup(t *testing.T) (socketClient client.OperationsCenterClient, unauthorizedHTTPClient client.OperationsCenterClient, db *sql.DB) {
	t.Helper()
	ctx := t.Context()

	logLevel := slog.LevelError
	if testing.Verbose() {
		logLevel = slog.LevelDebug
	}

	slog.SetDefault(
		slog.New(
			slog.NewTextHandler(
				os.Stderr,
				&slog.HandlerOptions{
					Level: logLevel,
				},
			),
		),
	)

	tmpDir := t.TempDir()

	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	cert, err := incustls.KeyPairFromRaw(certPEM, keyPEM)
	require.NoError(t, err)

	port := getFreeTCPPort(t)

	env := &mock.EnvironmentMock{
		GetUnixSocketFunc: func() string {
			return filepath.Join(tmpDir, "unix.socket")
		},
		VarDirFunc: func() string {
			return tmpDir
		},
		UsrShareDirFunc: func() string {
			return ""
		},
		IsIncusOSFunc: func() bool {
			return false
		},
		GetTokenFunc: func(ctx context.Context) (string, error) {
			return "", nil
		},
	}

	config.InitTest(t, env, nil, config.InternalConfig{
		IsBackgroundTasksDisabled: true,
		SourcePollSkipFirst:       true,
	})

	err = config.UpdateNetwork(ctx, shared.SystemNetworkPut{
		OperationsCenterAddress: "https://127.0.0.1:" + port,
		RestServerAddress:       "[::1]:" + port,
	})
	require.NoError(t, err)

	err = config.UpdateSecurity(ctx, shared.SystemSecurityPut{
		TrustedTLSClientCertFingerprints: []string{cert.Fingerprint()},
	})
	require.NoError(t, err)

	d := api.NewDaemon(
		ctx,
		env,
	)

	err = d.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		err = d.Stop(context.Background())
		require.NoError(t, err)
	})

	socketClient, err = client.New("http://unix.socket/", client.WithForceLocal(filepath.Join(tmpDir, "unix.socket")))
	require.NoError(t, err)
	unauthorizedHTTPClient, err = client.New("https://localhost:" + port) // without client.WithClientCertificate(cert)
	require.NoError(t, err)

	db, err = dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	return socketClient, unauthorizedHTTPClient, db
}

func getFreeTCPPort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "[::1]:0")
	require.NoError(t, err)

	defer func() {
		_ = l.Close()
	}()

	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return strconv.Itoa(addr.Port)
}

func noop(t *testing.T) {
	t.Helper()
}
