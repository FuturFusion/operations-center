package cluster

import (
	"testing"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	"github.com/stretchr/testify/require"
)

// nodeIDRepo is a global database only answering its node ID.
type nodeIDRepo struct {
	GlobalRaftRepo
}

func (nodeIDRepo) GetNodeID() int64 {
	return 1
}

// clusterDBGateway is a gateway only accepting the global database.
type clusterDBGateway struct {
	cowsqlcluster.Gateway
}

func (clusterDBGateway) SetClusterDB(cowsqldb.Cluster) {}

func TestMembership_SetGlobalDB_concurrent(t *testing.T) {
	t.Parallel()

	m := &membership{gateway: clusterDBGateway{}}
	m.SetGlobalDB(nodeIDRepo{})

	done := make(chan struct{})
	go func() {
		defer close(done)

		for range 100 {
			m.SetGlobalDB(nodeIDRepo{})
		}
	}()

	// Requests read the repository while startup sets it.
	for range 100 {
		require.Equal(t, int64(1), m.global().GetNodeID())
	}

	<-done
}
