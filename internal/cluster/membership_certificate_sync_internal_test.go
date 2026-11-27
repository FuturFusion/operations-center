package cluster

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"
)

// testKeyPair returns a generated certificate with its key.
func testKeyPair(t *testing.T) tls.Certificate {
	t.Helper()

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	keyPair, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
	require.NoError(t, err)

	return keyPair
}

// renewed returns a certificate for the key of keyPair valid from notBefore.
func renewed(t *testing.T, keyPair tls.Certificate, notBefore time.Time) *x509.Certificate {
	t.Helper()

	signer, ok := keyPair.PrivateKey.(crypto.Signer)
	require.True(t, ok)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(notBefore.UnixNano()),
		Subject:      keyPair.Leaf.Subject,
		NotBefore:    notBefore,
		NotAfter:     notBefore.Add(365 * 24 * time.Hour),
		DNSNames:     keyPair.Leaf.DNSNames,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, signer.Public(), signer)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert
}

func TestRenewedClusterCertificate(t *testing.T) {
	t.Parallel()

	local := testKeyPair(t)
	other := testKeyPair(t)

	require.True(t, sameKey(local.Leaf, local.Leaf))
	require.False(t, sameKey(local.Leaf, other.Leaf))

	// The same certificate and an older one for the key renew nothing.
	_, ok := renewedClusterCertificate([]*x509.Certificate{local.Leaf}, local.Leaf)
	require.False(t, ok)

	_, ok = renewedClusterCertificate([]*x509.Certificate{renewed(t, local, local.Leaf.NotBefore.Add(-time.Hour))}, local.Leaf)
	require.False(t, ok)

	// A newer certificate of another key can't be adopted without its key.
	_, ok = renewedClusterCertificate([]*x509.Certificate{renewed(t, other, time.Now())}, local.Leaf)
	require.False(t, ok)

	_, ok = renewedClusterCertificate(nil, local.Leaf)
	require.False(t, ok)

	// A newer certificate for the key is adopted with its chain.
	newer := renewed(t, local, local.Leaf.NotBefore.Add(time.Hour))
	chainPEM, ok := renewedClusterCertificate([]*x509.Certificate{newer, other.Leaf}, local.Leaf)
	require.True(t, ok)

	block, rest := pem.Decode(chainPEM)
	require.NotNil(t, block)
	require.Equal(t, newer.Raw, block.Bytes)

	block, rest = pem.Decode(rest)
	require.NotNil(t, block)
	require.Equal(t, other.Leaf.Raw, block.Bytes)
	require.Empty(t, rest)

	// The renewed certificate and the kept key still form a key pair.
	_, err := tls.X509KeyPair(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustMarshalKey(t, local.PrivateKey)}))
	require.NoError(t, err)
}

func mustMarshalKey(t *testing.T, key crypto.PrivateKey) []byte {
	t.Helper()

	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	return der
}

// raftNodesRepo is a raft node repo answering the raft nodes only.
type raftNodesRepo struct {
	cowsqldb.Node

	nodes []cowsqldb.RaftNode
}

func (r raftNodesRepo) GetRaftNodes(context.Context) ([]cowsqldb.RaftNode, error) {
	return r.nodes, nil
}

func TestSyncClusterCertificate(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = root.Close() })

	local := testKeyPair(t)
	localPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: local.Leaf.Raw})
	require.NoError(t, root.WriteFile("cluster.crt", localPEM, 0o644))

	presented := map[string][]*x509.Certificate{}
	remoteCertificates = func(address string) ([]*x509.Certificate, error) {
		chain, ok := presented[address]
		if !ok {
			return nil, errors.New("connection refused")
		}

		return chain, nil
	}

	t.Cleanup(func() {
		remoteCertificates = func(address string) ([]*x509.Certificate, error) {
			return incustls.GetRemoteCertificates("https://"+address, "")
		}
	})

	repo := raftNodesRepo{nodes: []cowsqldb.RaftNode{
		{Address: "192.0.2.1:8443", Role: cowsqldb.RaftVoter},
		{Address: "192.0.2.2:8443", Role: cowsqldb.RaftVoter},
		{Address: "192.0.2.3:8443", Role: cowsqldb.RaftVoter},
	}}

	// A standalone server has nothing to adopt.
	require.NoError(t, SyncClusterCertificate(t.Context(), root, raftNodesRepo{}, "192.0.2.1:8443"))

	// Members presenting the same certificate, an unreachable member and a certificate of another key change nothing.
	other := testKeyPair(t)
	presented["192.0.2.2:8443"] = []*x509.Certificate{local.Leaf}
	presented["192.0.2.3:8443"] = []*x509.Certificate{renewed(t, other, time.Now())}
	require.NoError(t, SyncClusterCertificate(t.Context(), root, repo, "192.0.2.1:8443"))

	got, err := root.ReadFile("cluster.crt")
	require.NoError(t, err)
	require.Equal(t, localPEM, got)

	// The newest certificate of the key among the members is adopted.
	newer := renewed(t, local, local.Leaf.NotBefore.Add(time.Hour))
	newest := renewed(t, local, local.Leaf.NotBefore.Add(2*time.Hour))
	presented["192.0.2.2:8443"] = []*x509.Certificate{newest}
	presented["192.0.2.3:8443"] = []*x509.Certificate{newer}
	require.NoError(t, SyncClusterCertificate(t.Context(), root, repo, "192.0.2.1:8443"))

	got, err = root.ReadFile("cluster.crt")
	require.NoError(t, err)
	block, _ := pem.Decode(got)
	require.NotNil(t, block)
	require.Equal(t, newest.Raw, block.Bytes)

	// The key still matches.
	key, err := root.ReadFile("cluster.key")
	require.ErrorIs(t, err, os.ErrNotExist, "the key is not touched")
	_ = key
}
