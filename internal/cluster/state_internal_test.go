package cluster

import (
	"crypto/tls"
	"net"
	"os"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/api/listener"
)

func TestState_NewClusterCertificate(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = root.Close() })

	// A stale CA of a previous cluster is removed.
	require.NoError(t, root.WriteFile("cluster.ca", []byte("stale"), 0o600))

	s := &state{varRoot: root}
	certInfo, err := s.NewClusterCertificate()
	require.NoError(t, err)

	_, err = root.Stat("cluster.ca")
	require.ErrorIs(t, err, os.ErrNotExist)

	certPEM, err := root.ReadFile("cluster.crt")
	require.NoError(t, err)

	keyPEM, err := root.ReadFile("cluster.key")
	require.NoError(t, err)

	stored, err := incustls.KeyPairFromRaw(certPEM, keyPEM)
	require.NoError(t, err)
	require.Equal(t, certInfo.Fingerprint(), stored.Fingerprint())
}

func TestState_SetClusterCertificate(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = root.Close() })

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, true)
	require.NoError(t, err)

	certInfo, err := incustls.KeyPairFromRaw(certPEM, keyPEM)
	require.NoError(t, err)

	s := &state{varRoot: root, listeners: &listener.ListenerSet{}}
	stored, err := s.SetClusterCertificate(certInfo)
	require.NoError(t, err)
	require.Equal(t, certInfo.Fingerprint(), stored.Fingerprint())

	storedCertPEM, err := root.ReadFile("cluster.crt")
	require.NoError(t, err)
	require.Equal(t, certPEM, storedCertPEM)

	storedKeyPEM, err := root.ReadFile("cluster.key")
	require.NoError(t, err)
	require.Equal(t, keyPEM, storedKeyPEM)
}

func TestState_HasListener(t *testing.T) {
	t.Parallel()

	require.Error(t, (&state{}).HasListener(t.Context()))

	listeners := &listener.ListenerSet{}
	s := &state{listeners: listeners}
	require.Error(t, s.HasListener(t.Context()))

	inner, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := listener.NewFancyTLSListener(inner, tls.Certificate{})
	t.Cleanup(func() { _ = server.Close() })

	listeners.SetServerListener(server)
	require.NoError(t, s.HasListener(t.Context()))
}

func TestState_standalone(t *testing.T) {
	t.Parallel()

	s := &state{}
	require.NoError(t, s.UpdateAuthenticator(t.Context()))
	require.Empty(t, s.ClusterAddress())
	s.OnHeartbeatNotification(nil)
}
