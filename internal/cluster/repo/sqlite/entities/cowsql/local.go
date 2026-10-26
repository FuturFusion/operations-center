package cowsql

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cowsql/go-cowsql/cluster/db"
	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/sql/sqlite/query"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/shared/api"
)

// CreateFirstRaftNode adds a node to the current list of nodes that are part of the
// cowsql Raft cluster. It returns the ID of the newly inserted row.
func CreateFirstRaftNode(ctx context.Context, dbtx transaction.DBTX, address, name string) error {
	id, err := query.UpsertObject(ctx, dbtx, "raft_nodes", []string{"id", "address", "name"}, []any{int64(1), address, name})
	if err != nil {
		return fmt.Errorf("Failed to create first raft node: %w", err)
	}

	if id != 1 {
		return errors.New("could not set raft node ID to 1")
	}

	return nil
}

// DetermineRaftNode figures out what raft node ID and address we have, if any.
//
// This decision is based on the value of the cluster.https_address config key
// and on the rows in the raft_nodes table, both stored in the node-level
// SQLite database.
//
// The following rules are applied:
//
//   - If no cluster.https_address config key is set, this is a non-clustered node
//     and the returned RaftNode will have ID 1 but no address, to signal that
//     the node should setup an in-memory raft cluster where the node itself
//     is the only member and leader.
//
//   - If cluster.https_address config key is set, but there is no row in the
//     raft_nodes table, this is a brand new clustered node that is joining a
//     cluster, and same behavior as the previous case applies.
//
//   - If cluster.https_address config key is set and there is at least one row
//     in the raft_nodes table, then this node is considered a raft node if
//     cluster.https_address matches one of the rows in raft_nodes. In that case,
//     the matching db.RaftNode row is returned, otherwise nil.
func DetermineRaftNode(ctx context.Context, tx transaction.TX, address string) (*db.RaftNode, error) {
	if address == "" {
		return db.DefaultRaftNode(), nil
	}

	nodes, err := GetRaftNodes(ctx, tx)
	if err != nil {
		return nil, err
	}

	// If cluster.https_address and the raft_nodes table is not populated,
	// this must be a joining node.
	if len(nodes) == 0 {
		return db.DefaultRaftNode(), nil
	}

	// Try to find a matching node.
	for _, node := range nodes {
		if node.Address == address {
			if node.Address == "" {
				return db.DefaultRaftNode(), nil
			}

			return &node, nil
		}
	}

	return db.DefaultRaftNode(), nil
}

// GetRaftNodes returns information about all cluster members that are members of the
// cowsql Raft cluster (possibly including the local member). If this server
// is not running in clustered mode, an empty list is returned.
func GetRaftNodes(ctx context.Context, dbtx transaction.DBTX) ([]db.RaftNode, error) {
	nodes := []db.RaftNode{}

	sql := "SELECT id, address, role, name FROM raft_nodes ORDER BY id"
	err := query.Scan(ctx, dbtx, sql, func(scan func(dest ...any) error) error {
		node := db.RaftNode{}
		err := scan(&node.ID, &node.Address, &node.Role, &node.Name)
		if err != nil {
			return err
		}

		nodes = append(nodes, node)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch raft nodes: %w", err)
	}

	return nodes, nil
}

// GetRaftNode returns the address of the raft node with the given ID,
// if any matching row exists.
func GetRaftNode(ctx context.Context, dbtx transaction.DBTX, id int64) (*db.RaftNode, error) {
	nodes := []db.RaftNode{}

	sql := "SELECT id, address, role, name FROM raft_nodes WHERE id = ? ORDER BY id"
	err := query.Scan(ctx, dbtx, sql, func(scan func(dest ...any) error) error {
		node := db.RaftNode{}
		err := scan(&node.ID, &node.Address, &node.Role, &node.Name)
		if err != nil {
			return err
		}

		nodes = append(nodes, node)

		return nil
	}, id)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch raft nodes: %w", err)
	}

	switch len(nodes) {
	case 0:
		return nil, api.StatusErrorf(http.StatusNotFound, "Raft member not found")

	case 1:
		return &nodes[0], nil

	default:
		// This should never happen since we have a UNIQUE constraint
		// on the raft_nodes.id column.
		return nil, errors.New("more than one match found")
	}
}

// ReplaceRaftNodes replaces the current list of raft nodes.
func ReplaceRaftNodes(ctx context.Context, tx transaction.TX, nodes []db.RaftNode) error {
	certsByAddr, err := GetRaftCertificates(ctx, tx)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM raft_nodes")
	if err != nil {
		return err
	}

	columns := []string{"id", "name", "address", "certificate", "role"}
	for _, node := range nodes {
		values := []any{node.ID, node.Name, node.Address, nil, node.Role}
		cert, ok := certsByAddr[node.Address]
		if ok {
			values = []any{node.ID, node.Name, node.Address, certificate.X509EncodeToPEM(cert.Certificate), node.Role}
		}

		_, err := query.UpsertObject(ctx, tx, "raft_nodes", columns, values)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetRaftCertificates is a helper to fetch persisted server certificates from the local cache of cluster members.
func GetRaftCertificates(ctx context.Context, dbtx transaction.DBTX) (map[string]api.Certificate, error) {
	certsByAddr := map[string]api.Certificate{}

	var scan query.Dest = func(scan func(dest ...any) error) error {
		var address string
		var certPEM string

		err := scan(&address, &certPEM)
		if err != nil {
			return err
		}

		cert, err := certificate.Decode([]byte(certPEM))
		if err != nil {
			return fmt.Errorf("Failed to decode certificate of raft member %q: %w", address, err)
		}

		certsByAddr[address] = api.Certificate{Certificate: cert}

		return nil
	}

	err := query.SelectObjectsRaw(ctx, dbtx, "SELECT address, certificate FROM raft_nodes WHERE address != '1' AND certificate IS NOT NULL", scan)
	if err != nil {
		return nil, err
	}

	return certsByAddr, nil
}

// SetRaftCertificateByAddress is a helper to fetch persisted server certificates from the local cache of cluster members.
func SetRaftCertificateByAddress(ctx context.Context, dbtx transaction.DBTX, cert api.Certificate, address string) error {
	_, err := dbtx.ExecContext(ctx, "UPDATE raft_nodes SET certificate = ? WHERE address = ?", certificate.X509EncodeToPEM(cert.Certificate), address)

	return err
}
