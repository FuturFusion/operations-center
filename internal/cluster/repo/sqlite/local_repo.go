package sqlite

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"time"

	"github.com/cowsql/go-cowsql/cluster/db"
	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities/cowsql"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

type raft struct {
	db         transaction.DBTX
	internalDB *sql.DB
	dir        string
	system     system.SystemService
}

type LocalDB struct {
	*sql.DB
}

func (l *LocalDB) BeginTx(ctx context.Context) (transaction.TX, error) {
	return l.DB.BeginTx(ctx, nil)
}

func NewRaftRepo(internalDB *LocalDB, dir string, s system.SystemService) *raft {
	return &raft{db: transaction.Enable(internalDB), internalDB: internalDB.DB, dir: dir, system: s}
}

// MaxRetries implements [cluster.LocalRaftRepo].
func (r *raft) MaxRetries() int {
	return 0
}

// OnTxStart implements [cluster.LocalRaftRepo].
func (r *raft) OnTxStart(exclusive bool) func() {
	return nil
}

func (r *raft) TxTimeout() time.Duration {
	return 0
}

// DBTX implements [cluster.LocalRaftRepo].
func (r *raft) DBTX() transaction.DBTX {
	return r.db
}

func (r *raft) EnterExclusive() error {
	return nil
}

// DB implements [cluster.LocalRaftRepo].
func (r *raft) DB() *sql.DB {
	return r.internalDB
}

// GlobalDatabaseDir implements [cluster.LocalRaftRepo].
func (r *raft) GlobalDatabaseDir() string {
	return filepath.Join(r.dir, "global")
}

// LogPath implements [cluster.LocalRaftRepo].
func (r *raft) LogPath() string {
	return filepath.Join(r.GlobalDatabaseDir(), "logs.db")
}

// GetClusterAddress implements [cluster.LocalRaftRepo].
func (r *raft) GetClusterAddress(ctx context.Context) (string, error) {
	n := r.system.GetNetworkConfig(ctx)
	return n.ClusterAddress, nil
}

// SetClusterAddress implements [cluster.LocalRaftRepo].
func (r *raft) SetClusterAddress(ctx context.Context, address string) error {
	n := r.system.GetNetworkConfig(ctx)
	n.ClusterAddress = address
	return r.system.UpdateNetworkConfig(ctx, apisystem.NetworkFullPut{
		NetworkPut:       n.NetworkPut,
		NetworkGlobalPut: n.NetworkGlobalPut,
	})
}

// CreateFirstRaftNode implements [cluster.LocalRaftRepo].
func (r *raft) CreateFirstRaftNode(ctx context.Context, address string, name string) error {
	return transaction.ForceTx(ctx, r, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.CreateFirstRaftNode(ctx, tx, address, name)
	})
}

// DetermineRaftNode implements [cluster.LocalRaftRepo].
func (r *raft) DetermineRaftNode(ctx context.Context) (*db.RaftNode, error) {
	address, err := r.GetClusterAddress(ctx)
	if err != nil {
		return nil, err
	}

	var n *db.RaftNode
	err = transaction.ForceTx(ctx, r, func(ctx context.Context, tx transaction.TX) error {
		var err error
		n, err = cowsql.DetermineRaftNode(ctx, tx, address)
		return err
	})
	if err != nil {
		return nil, err
	}

	return n, nil
}

// GetRaftNode implements [cluster.LocalRaftRepo].
func (r *raft) GetRaftNode(ctx context.Context, id int64) (*db.RaftNode, bool, error) {
	var node *db.RaftNode
	err := transaction.ForceTx(ctx, r, func(ctx context.Context, tx transaction.TX) error {
		var err error
		node, err = cowsql.GetRaftNode(ctx, tx, id)
		return err
	})
	if err != nil && !api.StatusErrorCheck(err, http.StatusNotFound) {
		return nil, false, err
	}

	return node, err == nil, nil
}

// GetRaftNodes implements [cluster.LocalRaftRepo].
func (r *raft) GetRaftNodes(ctx context.Context) ([]db.RaftNode, error) {
	var ns []db.RaftNode
	err := transaction.ForceTx(ctx, r, func(ctx context.Context, tx transaction.TX) error {
		var err error
		ns, err = cowsql.GetRaftNodes(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}

	return ns, nil
}

// ReplaceRaftNodes implements [cluster.LocalRaftRepo].
func (r *raft) ReplaceRaftNodes(ctx context.Context, nodes []db.RaftNode) error {
	return transaction.ForceTx(ctx, r, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.ReplaceRaftNodes(ctx, tx, nodes)
	})
}

func (r *raft) GetNetworkConfig(ctx context.Context) apisystem.Network {
	return r.system.GetNetworkConfig(ctx).Network
}

func (r *raft) GetCertificates(ctx context.Context) (map[string]api.Certificate, error) {
	var certs map[string]api.Certificate
	err := transaction.ForceTx(ctx, r, func(ctx context.Context, tx transaction.TX) error {
		var err error
		certs, err = cowsql.GetRaftCertificates(ctx, tx)

		return err
	})
	if err != nil {
		return nil, err
	}

	return certs, nil
}

func (r *raft) SetRaftCertificateByAddress(ctx context.Context, cert api.Certificate, address string) error {
	return transaction.ForceTx(ctx, r, func(ctx context.Context, tx transaction.TX) error {
		return cowsql.SetRaftCertificateByAddress(ctx, tx, cert, address)
	})
}
