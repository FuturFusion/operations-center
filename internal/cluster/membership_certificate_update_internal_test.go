package cluster

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync"
	"testing"

	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/api/listener"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func TestMembership_UpdateClusterCertificate_putsBackOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	root := fsroot.Open(t, dir)

	currentPEM, currentKeyPEM := testKeyPairPEM(t)
	newPEM, newKeyPEM := testKeyPairPEM(t)
	current, err := tls.X509KeyPair(currentPEM, currentKeyPEM)
	require.NoError(t, err)
	require.NoError(t, root.WriteFile("cluster.crt", currentPEM, 0o644))
	require.NoError(t, root.WriteFile("cluster.key", currentKeyPEM, 0o600))

	g := &memberGateway{
		st:        &state{varRoot: root, listeners: &listener.ListenerSet{}},
		cert:      cowsqltls.NewCertInfo(current, nil, nil),
		addresses: []string{"192.0.2.2:8443", "192.0.2.3:8443"},
	}

	fingerprint := func(pem string) string {
		cert, err := incustls.KeyPairFromRaw([]byte(pem), newKeyPEM)
		if err != nil {
			cert, err = incustls.KeyPairFromRaw([]byte(pem), currentKeyPEM)
		}

		require.NoError(t, err)

		return cert.Fingerprint()
	}

	// The members present the certificate they applied last. The second member applies the new certificate, but its answer is lost.
	var mu sync.Mutex
	serving := map[string]string{"192.0.2.2:8443": fingerprint(string(currentPEM)), "192.0.2.3:8443": fingerprint(string(currentPEM))}
	var posts []string
	lostAnswer := true

	m := &membership{
		gateway: g,
		varRoot: root,
		postCertificate: func(_ context.Context, address string, trusted *x509.Certificate, post apisystem.CertificatePost) error {
			mu.Lock()
			defer mu.Unlock()

			if incustls.CertFingerprint(trusted) != serving[address] {
				return errors.New("Remote certificate differs from expected")
			}

			serving[address] = fingerprint(post.Certificate)
			posts = append(posts, address+" "+serving[address][:8])

			if address == "192.0.2.3:8443" && lostAnswer {
				lostAnswer = false
				return errors.New("Internal server error")
			}

			return nil
		},
	}

	err = m.UpdateClusterCertificate(t.Context(), string(newPEM), string(newKeyPEM))
	require.ErrorIs(t, err, domain.ErrUnavailable)

	// The new certificate reached both members and the previous one is back on both, this member never changed.
	current8, new8 := fingerprint(string(currentPEM))[:8], fingerprint(string(newPEM))[:8]
	require.ElementsMatch(t, []string{"192.0.2.2:8443 " + new8, "192.0.2.3:8443 " + new8, "192.0.2.2:8443 " + current8, "192.0.2.3:8443 " + current8}, posts)
	require.Equal(t, fingerprint(string(currentPEM)), serving["192.0.2.2:8443"])
	require.Equal(t, fingerprint(string(currentPEM)), serving["192.0.2.3:8443"])

	stored, err := root.ReadFile("cluster.crt")
	require.NoError(t, err)
	require.Equal(t, currentPEM, stored)
}
