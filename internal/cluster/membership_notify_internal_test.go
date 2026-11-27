package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	cowstate "github.com/cowsql/go-cowsql/cluster/state"
	cowsqltls "github.com/cowsql/go-cowsql/cluster/tls"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
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

func (g *memberGateway) HeartbeatOfflineThreshold() time.Duration { return 20 * time.Second }

// memberNodes is a global database knowing the members and their heartbeats.
type memberNodes struct {
	GlobalRaftRepo

	nodes []cowsqldb.NodeInfo
}

func (n *memberNodes) GetNodes(context.Context) ([]cowsqldb.NodeInfo, error) { return n.nodes, nil }

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

func TestMembership_reloadSharedConfigOnMembers_skipsMembersCountedOffline(t *testing.T) {
	t.Parallel()

	g := &memberGateway{addresses: []string{"192.0.2.2:8443", "192.0.2.3:8443"}}

	// The third member stopped sending heartbeats a while ago.
	var repo GlobalRaftRepo = &memberNodes{nodes: []cowsqldb.NodeInfo{
		{Address: "192.0.2.1:8443", Heartbeat: time.Now()},
		{Address: "192.0.2.2:8443", Heartbeat: time.Now()},
		{Address: "192.0.2.3:8443", Heartbeat: time.Now().Add(-time.Minute)},
	}}

	var mu sync.Mutex
	var asked []string
	answers := map[string]error{
		"192.0.2.2:8443": nil,
		"192.0.2.3:8443": errors.New("dial tcp 192.0.2.3:8443: connect: connection refused"),
	}

	m := &membership{
		gateway: g,
		reloadConfigOnMember: func(_ context.Context, address string) error {
			mu.Lock()
			defer mu.Unlock()

			asked = append(asked, address)

			return answers[address]
		},
	}
	m.globalRepo.Store(&repo)

	// The member counted as offline is asked as well, and applies the config when it starts if it fails, so the change stands.
	require.NoError(t, m.reloadSharedConfigOnMembers(t.Context()))
	require.ElementsMatch(t, []string{"192.0.2.2:8443", "192.0.2.3:8443"}, asked)

	// A member counted as online fails the publish by failing to reload, whatever the reason.
	answers["192.0.2.2:8443"] = &client.ServerError{StatusCode: 500, Message: "Failed to apply"}
	require.Error(t, m.reloadSharedConfigOnMembers(t.Context()))

	answers["192.0.2.2:8443"] = errors.New("net/http: TLS handshake timeout")
	require.Error(t, m.reloadSharedConfigOnMembers(t.Context()))
}
