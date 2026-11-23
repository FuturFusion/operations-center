package cluster

import (
	"context"
	"testing"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	cowstate "github.com/cowsql/go-cowsql/cluster/state"
	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"
)

// memberGateway is a gateway with a cluster certificate and the other members to notify.
type memberGateway struct {
	cowsqlcluster.Gateway

	st        *state
	cert      cowsqltls.CertInfo
	addresses []string
}

func (g *memberGateway) State() cowstate.State { return g.st }

func (g *memberGateway) NetworkCert() cowsqltls.CertInfo { return g.cert }

func (g *memberGateway) ServerCert() cowsqltls.CertInfo { return g.cert }

func (g *memberGateway) NetworkUpdateCert(cert cowsqltls.CertInfo) { g.cert = cert }

func (g *memberGateway) NewNotifier(context.Context, cowsqltls.CertInfo, cowsqltls.CertInfo, cowsqlcluster.NotifierPolicy) (cowsqlcluster.Notifier, error) {
	return func(hook func(ctx context.Context, address string, networkCert, serverCert cowsqltls.CertInfo) error) []error {
		var errs []error
		for _, address := range g.addresses {
			err := hook(context.Background(), address, g.cert, g.cert)
			if err != nil {
				errs = append(errs, err)
			}
		}

		return errs
	}, nil
}

// testKeyPairPEM returns a generated certificate with its key, PEM encoded.
func testKeyPairPEM(t *testing.T) (certPEM []byte, keyPEM []byte) {
	t.Helper()

	certPEM, keyPEM, err := incustls.GenerateMemCert(false, true)
	require.NoError(t, err)

	return certPEM, keyPEM
}
