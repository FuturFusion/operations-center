package cowsql

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cowsql/go-cowsql/cluster/db"
	"github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite/query"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/shared/api"
)

// APINodeInfo holds information about a single member in a cluster.
type APINodeInfo struct {
	db.NodeInfo
}

const (
	ClusterMemberStateDefault = 0
	clusterMemberStatePending = 1
)

// Version returns the node's version, composed by its schema level and
// number of extensions.
func (n APINodeInfo) Version() [2]int {
	return [2]int{n.Schema, n.APIExtensions}
}

// BootstrapNode sets the name and address of the first cluster member, with id: 1.
func BootstrapNode(ctx context.Context, tx transaction.DBTX, name string, address string) error {
	result, err := tx.ExecContext(ctx, "UPDATE cluster_members SET name=?, address=? WHERE id=1", name, address)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n != 1 {
		return fmt.Errorf("query updated %d rows instead of 1", n)
	}

	return nil
}

// CreateNode creates the cluster member record in the global database with the given properties.
func CreateNode(ctx context.Context, tx transaction.DBTX, name string, address string, arch int) (int64, error) {
	// TODO: API extensions.
	columns := []string{"name", "address", "schema", "api_extensions", "description", "architecture"}
	values := []any{name, address, dbschema.SchemaVersion, 0, "", arch}
	return query.UpsertObject(ctx, tx, "cluster_members", columns, values)
}

// GetNodeByName returns the node with the given name.
func GetNodeByName(ctx context.Context, tx transaction.TX, name string, pending bool) (db.NodeInfo, error) {
	null := db.NodeInfo{}
	nodes, err := nodes(ctx, tx, pending, "name=?", name)
	if err != nil {
		return null, err
	}

	switch len(nodes) {
	case 0:
		return null, api.StatusErrorf(http.StatusNotFound, "Cluster member not found")

	case 1:
		return nodes[0], nil

	default:
		return null, errors.New("More than one cluster member matches")
	}
}

// GetNodeByAddress returns the node with the given network address.
func GetNodeByAddress(ctx context.Context, tx transaction.TX, address string, pending bool) (db.NodeInfo, error) {
	null := db.NodeInfo{}
	nodes, err := nodes(ctx, tx, pending, "address=?", address)
	if err != nil {
		return null, err
	}

	switch len(nodes) {
	case 0:
		return null, api.StatusErrorf(http.StatusNotFound, "Cluster member not found")

	case 1:
		return nodes[0], nil

	default:
		return null, errors.New("More than one cluster member matches")
	}
}

// GetNodes returns all cluster members that are part of the cluster.
func GetNodes(ctx context.Context, tx transaction.TX, pending bool) ([]db.NodeInfo, error) {
	return nodes(ctx, tx, pending, "")
}

// GetNodesCount returns the number of members in the cluster.
//
// Since there's always at least one node row, even when not-clustered, the
// return value is greater than zero.
func GetNodesCount(ctx context.Context, tx transaction.DBTX) (int, error) {
	count, err := query.Count(ctx, tx, "cluster_members", "")
	if err != nil {
		return 0, fmt.Errorf("failed to count existing cluster members: %w", err)
	}

	return count, nil
}

// GetNodesFailureDomains returns a map associating each node address with its
// failure domain code.
func GetNodesFailureDomains(ctx context.Context, tx transaction.DBTX) (map[string]uint64, error) {
	stmt := "SELECT address, failure_domain FROM cluster_members"
	type failureDomain struct {
		address string
		ID      int64 `json:"id"`
	}

	rows := []failureDomain{}
	err := query.Scan(ctx, tx, stmt, func(scan func(dest ...any) error) error {
		var address, fd string
		err := scan(&address, &fd)
		if err != nil {
			return err
		}

		var obj failureDomain
		err = json.Unmarshal([]byte(fd), &obj)
		if err != nil {
			return err
		}

		obj.address = address

		rows = append(rows, obj)

		return nil
	})
	if err != nil {
		return nil, err
	}

	domains := map[string]uint64{}

	for _, row := range rows {
		domains[row.address] = uint64(row.ID)
	}

	return domains, nil
}

// NodeIsOutdated returns true if there's some cluster node having an API or
// schema version greater than the node this method is invoked on.
func NodeIsOutdated(ctx context.Context, tx transaction.TX, nodeID int64) (bool, error) {
	nodes, err := nodes(ctx, tx, false /* not pending */, "")
	if err != nil {
		return false, fmt.Errorf("Failed to fetch cluster members: %w", err)
	}

	// Figure our own version.
	ver := [2]int{}
	for _, node := range nodes {
		if node.ID == nodeID {
			ver = APINodeInfo{NodeInfo: node}.Version()
		}
	}

	// FIXME: Check API Extensions (other index).
	if ver[0] == 0 {
		return false, errors.New("Inconsistency: local member not found")
	}

	// Check if any of the other nodes is greater than us.
	for _, node := range nodes {
		if node.ID == nodeID {
			continue
		}

		n, err := dbschema.CompareVersions(APINodeInfo{NodeInfo: node}.Version(), ver, true)
		if err != nil {
			return false, fmt.Errorf("Failed to compare with version of member %s: %w", node.Name, err)
		}

		if n == 1 {
			// The other node's version is greater than ours.
			return true, nil
		}
	}

	return false, nil
}

// RemoveNode removes the node with the given name.
func RemoveNode(ctx context.Context, tx transaction.DBTX, name string) error {
	result, err := tx.ExecContext(ctx, "DELETE FROM cluster_members WHERE name=?", name)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n != 1 {
		return fmt.Errorf("query deleted %d rows instead of 1", n)
	}

	return nil
}

// SetNodeHeartbeat updates the heartbeat column of the node with the given address.
func SetNodeHeartbeat(ctx context.Context, tx transaction.DBTX, address string, heartbeat time.Time) error {
	stmt := "UPDATE cluster_members SET heartbeat=? WHERE address=?"
	result, err := tx.ExecContext(ctx, stmt, heartbeat, address)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n < 1 {
		return api.StatusErrorf(http.StatusNotFound, "Cluster member not found")
	} else if n > 1 {
		return fmt.Errorf("Expected to update one row and not %d", n)
	}

	return nil
}

// SetNodePendingFlag toggles the pending flag for the node. A node is pending when
// it's been accepted in the cluster, but has not yet actually joined it.
func SetNodePendingFlag(ctx context.Context, tx transaction.DBTX, id int64, pending bool) error {
	value := ClusterMemberStateDefault
	if pending {
		value = clusterMemberStatePending
	}

	result, err := tx.ExecContext(ctx, "UPDATE cluster_members SET state=? WHERE id=?", value, id)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n != 1 {
		return fmt.Errorf("query updated %d rows instead of 1", n)
	}

	return nil
}

// SetNodeCertificateByName sets the certificate for the cluster member with the given name.
func SetNodeCertificateByName(ctx context.Context, tx transaction.TX, serverName string, serverCert *x509.Certificate) error {
	result, err := tx.ExecContext(ctx, "UPDATE cluster_members SET certificate=? WHERE name=?", certificate.X509EncodeToPEM(serverCert), serverName)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n != 1 {
		return fmt.Errorf("Failed to set certificate, expected to update one row and not %d", n)
	}

	return nil
}

// Nodes returns all members part of the cluster.
func nodes(ctx context.Context, tx transaction.TX, pending bool, where string, args ...any) ([]db.NodeInfo, error) {
	// Get the node entries
	stmt := "SELECT id, name, address, description, schema, api_extensions, heartbeat, state, config, roles FROM cluster_members "

	if pending {
		// Include only pending nodes
		stmt += "WHERE state=? "
	} else {
		// Include created and evacuated nodes
		stmt += "WHERE state!=? "
	}

	args = append([]any{clusterMemberStatePending}, args...)

	if where != "" {
		stmt += fmt.Sprintf("AND %s ", where)
	}

	stmt += "ORDER BY id"

	// Process node entries.
	// refTime anchors the IsOffline check to the time the heartbeat snapshot
	// was taken (see NodeInfo.heartbeatRefTime).
	refTime := time.Now()
	nodes := []db.NodeInfo{}
	err := query.Scan(ctx, tx, stmt, func(scan func(dest ...any) error) error {
		node := db.NodeInfo{}
		node.SetHeartbeatRefTime(refTime)

		var config, roles string
		err := scan(&node.ID, &node.Name, &node.Address, &node.Description, &node.Schema, &node.APIExtensions, &node.Heartbeat, &node.State, &config, &roles)
		if err != nil {
			return err
		}

		err = json.Unmarshal([]byte(config), &node.Config)
		if err != nil {
			return err
		}

		err = json.Unmarshal([]byte(roles), &node.Roles)
		if err != nil {
			return err
		}

		nodes = append(nodes, node)

		return nil
	}, args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch cluster members: %w", err)
	}

	return nodes, nil
}

// GetClusterCertificates is a helper to fetch persisted server certificates from the local cache of cluster members.
func GetClusterCertificates(ctx context.Context, dbtx transaction.DBTX) (map[string]api.Certificate, error) {
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
			return fmt.Errorf("Failed to decode certificate of cluster member %q: %w", address, err)
		}

		certsByAddr[address] = api.Certificate{Certificate: cert}

		return nil
	}

	err := query.SelectObjectsRaw(ctx, dbtx, "SELECT address, certificate FROM cluster_members WHERE address != '0.0.0.0' AND certificate IS NOT NULL", scan)
	if err != nil {
		return nil, err
	}

	return certsByAddr, nil
}
