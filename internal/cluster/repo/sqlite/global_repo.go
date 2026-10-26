package sqlite

import (
	"context"
	"crypto/x509"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/cowsql/go-cowsql/cluster/db"
	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities/cowsql"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	cowsqlcluster "github.com/FuturFusion/operations-center/internal/sql/cowsql"
	"github.com/FuturFusion/operations-center/shared/api"
)

type globalDB struct {
	mu         sync.RWMutex
	db         transaction.DBTX
	internalDB *cowsqlcluster.GlobalDB
}

func NewGlobalRaftRepo(g *cowsqlcluster.GlobalDB) *globalDB {
	return &globalDB{db: transaction.Enable(g), internalDB: g}
}

// MaxRetries implements [db.Cluster].
func (g *globalDB) MaxRetries() int {
	return 250
}

// OnTxStart implements [db.Cluster].
func (g *globalDB) OnTxStart(exclusive bool) func() {
	if !exclusive {
		g.mu.RLock()
	}

	cleanup := func() {
		if exclusive {
			g.mu.Unlock()
		} else {
			g.mu.RUnlock()
		}
	}

	return cleanup
}

// TxTimeout implements [transaction.Transactor].
func (g *globalDB) TxTimeout() time.Duration {
	return 30 * time.Second
}

// DBTX implements [cluster.LocalRaftRepo].
func (g *globalDB) DBTX() transaction.DBTX {
	return g.db
}

// EnterExclusive implements [db.Cluster].
func (g *globalDB) EnterExclusive() error {
	ch := make(chan struct{})
	go func() {
		g.mu.Lock()
		ch <- struct{}{}
	}()

	timeout := 20 * time.Second
	select {
	case <-ch:
		return nil

	case <-time.After(timeout):
		return fmt.Errorf("timeout (%s)", timeout)
	}
}

func (g *globalDB) DB() *sql.DB {
	return g.internalDB.DB
}

// BootstrapNode implements [db.Cluster].
func (g *globalDB) BootstrapNode(ctx context.Context, serverName string, clusterAddress string) error {
	return transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.BootstrapNode(ctx, tx, serverName, clusterAddress)
	})
}

// CreateNode implements [db.Cluster].
func (g *globalDB) CreateNode(ctx context.Context, name string, address string, arch int) (int64, error) {
	var id int64
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		id, err = cowsql.CreateNode(ctx, tx, name, address, arch)
		return err
	})
	if err != nil {
		return -1, err
	}

	return id, nil
}

// GetNodeByName implements [db.Cluster].
func (g *globalDB) GetNodeByName(ctx context.Context, name string, pending bool) (db.NodeInfo, error) {
	var n db.NodeInfo
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		n, err = cowsql.GetNodeByName(ctx, tx, name, pending)
		return err
	})
	if err != nil {
		return db.NodeInfo{}, err
	}

	return n, nil
}

// GetNodeByAddress implements [db.Cluster].
func (g *globalDB) GetNodeByAddress(ctx context.Context, address string, pending bool) (db.NodeInfo, error) {
	var n db.NodeInfo
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		n, err = cowsql.GetNodeByAddress(ctx, tx, address, pending)
		return err
	})
	if err != nil {
		return db.NodeInfo{}, err
	}

	return n, nil
}

// GetNodeOfflineThreshold implements [db.Cluster].
func (g *globalDB) GetNodeOfflineThreshold(ctx context.Context) (time.Duration, error) {
	var t time.Duration
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		t = time.Duration(config.GetCluster().OfflineThresholdSeconds) * time.Second
		return nil
	})
	if err != nil {
		return -1, err
	}

	return t, nil
}

// GetNodes implements [db.Cluster].
func (g *globalDB) GetNodes(ctx context.Context) ([]db.NodeInfo, error) {
	var n []db.NodeInfo
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		n, err = cowsql.GetNodes(ctx, tx, false)
		return err
	})
	if err != nil {
		return nil, err
	}

	return n, nil
}

// GetNodesCount implements [db.Cluster].
func (g *globalDB) GetNodesCount(ctx context.Context) (int, error) {
	var count int
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		count, err = cowsql.GetNodesCount(ctx, tx)
		return err
	})
	if err != nil {
		return -1, err
	}

	return count, nil
}

// GetNodesFailureDomains implements [db.Cluster].
func (g *globalDB) GetNodesFailureDomains(ctx context.Context) (map[string]uint64, error) {
	var fds map[string]uint64
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		fds, err = cowsql.GetNodesFailureDomains(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}

	return fds, nil
}

// NodeIsOutdated implements [db.Cluster].
func (g *globalDB) NodeIsOutdated(ctx context.Context) (bool, error) {
	var outdated bool
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		outdated, err = cowsql.NodeIsOutdated(ctx, tx, g.internalDB.NodeID())
		return err
	})
	if err != nil {
		return false, err
	}

	return outdated, nil
}

// RemoveNode implements [db.Cluster].
func (g *globalDB) RemoveNode(ctx context.Context, name string) error {
	return transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.RemoveNode(ctx, tx, name)
	})
}

// SetNodeHeartbeat implements [db.Cluster].
func (g *globalDB) SetNodeHeartbeat(ctx context.Context, address string, heartbeatTime time.Time) error {
	return transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.SetNodeHeartbeat(ctx, tx, address, heartbeatTime)
	})
}

// SetNodePendingFlag implements [db.Cluster].
func (g *globalDB) SetNodePendingFlag(ctx context.Context, nodeID int64, flag bool) error {
	if !flag {
		g.internalDB.SetNodeID(nodeID)
	}

	return transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.SetNodePendingFlag(ctx, tx, nodeID, flag)
	})
}

// SetNodeCertificateByName implements [db.Cluster].
func (g *globalDB) SetNodeCertificateByName(ctx context.Context, serverName string, serverCert *x509.Certificate) error {
	return transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.SetNodeCertificateByName(ctx, tx, serverName, serverCert)
	})
}

func (g *globalDB) SchemaVersion() int {
	return g.internalDB.SchemaVersion()
}

func (g *globalDB) GetNodeID() int64 {
	return g.internalDB.NodeID()
}

func (g *globalDB) DetermineNodeID(ctx context.Context, address string) error {
	return transaction.Do(ctx, g, func(ctx context.Context) error {
		return transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
			members, err := cowsql.GetNodes(ctx, tx, false)
			if err != nil {
				return fmt.Errorf("Failed getting cluster members: %w", err)
			}

			memberID := int64(-1)
			if len(members) == 1 && members[0].Address == "0.0.0.0" {
				// We're not clustered
				memberID = 1
			} else {
				for _, member := range members {
					if member.Address == address {
						memberID = member.ID
						break
					}
				}
			}

			if memberID < 0 {
				return fmt.Errorf("No node registered with address %s", address)
			}

			// Set the local member ID
			g.internalDB.SetNodeID(memberID)

			return nil
		})
	})
}

func (g *globalDB) GetCertificates(ctx context.Context) (map[string]api.Certificate, error) {
	var certs map[string]api.Certificate
	err := transaction.ForceTx(ctx, g, func(ctx context.Context, tx transaction.TX) error {
		var err error
		certs, err = cowsql.GetClusterCertificates(ctx, tx)

		return err
	})
	if err != nil {
		return nil, err
	}

	return certs, nil
}
