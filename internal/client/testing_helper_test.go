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

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/api"
	"github.com/FuturFusion/operations-center/internal/client"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment/mock"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// testDaemon bundles a running Operations Center daemon together with the
// clients and the database handle necessary to drive it from a test.
type testDaemon struct {
	socketClient           client.OperationsCenterClient
	authorizedHTTPClient   client.OperationsCenterClient
	unauthorizedHTTPClient client.OperationsCenterClient
	db                     *sql.DB
	varDir                 string
}

func daemonSetup(t *testing.T) testDaemon {
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
		CacheDirFunc: func() string {
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

	err = config.UpdateNetwork(ctx, system.NetworkPut{
		OperationsCenterAddress: "https://127.0.0.1:" + port,
		RestServerAddress:       testingnet.LocalhostIP(t) + ":" + port,
	})
	require.NoError(t, err)

	err = config.UpdateSecurity(ctx, system.SecurityPut{
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

	socketClient, err := client.New("http://unix.socket/", client.WithForceLocal(filepath.Join(tmpDir, "unix.socket")))
	require.NoError(t, err)

	serverCert, err := incustls.ReadCert(filepath.Join(tmpDir, "server.crt"))
	require.NoError(t, err)

	authorizedHTTPClient, err := client.New("https://localhost:"+port, client.WithTrustedServerCertificate(serverCert), client.WithClientCertificate(cert))
	require.NoError(t, err)

	unauthorizedHTTPClient, err := client.New("https://localhost:"+port, client.WithTrustedServerCertificate(serverCert)) // without client.WithClientCertificate(cert)
	require.NoError(t, err)

	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	return testDaemon{
		socketClient:           socketClient,
		authorizedHTTPClient:   authorizedHTTPClient,
		unauthorizedHTTPClient: unauthorizedHTTPClient,
		db:                     db.DB,
		varDir:                 tmpDir,
	}
}

func getFreeTCPPort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", testingnet.LocalhostIP(t)+":0")
	require.NoError(t, err)

	defer func() {
		_ = l.Close()
	}()

	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return strconv.Itoa(addr.Port)
}

// seedFile writes content to the given path relative to the var directory of
// the daemon, creating any intermediate directories.
func seedFile(t *testing.T, d testDaemon, name string, content []byte) {
	t.Helper()

	filename := filepath.Join(d.varDir, name)

	err := os.MkdirAll(filepath.Dir(filename), 0o700)
	require.NoError(t, err)

	err = os.WriteFile(filename, content, 0o600)
	require.NoError(t, err)
}

func noop(t *testing.T) {
	t.Helper()
}
