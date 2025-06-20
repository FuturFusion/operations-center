// Code generated by generate-inventory; DO NOT EDIT.

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

type networkAddressSet struct {
	db sqlite.DBTX
}

var _ inventory.NetworkAddressSetRepo = &networkAddressSet{}

func NewNetworkAddressSet(db sqlite.DBTX) *networkAddressSet {
	return &networkAddressSet{
		db: db,
	}
}

func (r networkAddressSet) Create(ctx context.Context, in inventory.NetworkAddressSet) (inventory.NetworkAddressSet, error) {
	const sqlStmt = `
WITH _lookup AS (
  SELECT id AS cluster_id FROM clusters WHERE clusters.name = :cluster_name
)
INSERT INTO network_address_sets (uuid, cluster_id, project_name, name, object, last_updated)
VALUES (:uuid, (SELECT cluster_id FROM _lookup), :project_name, :name, :object, :last_updated)
RETURNING id, :uuid, :cluster_name, project_name, name, object, last_updated;
`

	marshaledObject, err := json.Marshal(in.Object)
	if err != nil {
		return inventory.NetworkAddressSet{}, err
	}

	row := r.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("uuid", in.UUID),
		sql.Named("cluster_name", in.Cluster),
		sql.Named("project_name", in.ProjectName),
		sql.Named("name", in.Name),
		sql.Named("object", marshaledObject),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return inventory.NetworkAddressSet{}, sqlite.MapErr(row.Err())
	}

	return scanNetworkAddressSet(row)
}

func (r networkAddressSet) GetAllWithFilter(ctx context.Context, filter inventory.NetworkAddressSetFilter) (inventory.NetworkAddressSets, error) {
	const sqlStmt = `
SELECT
  network_address_sets.id, network_address_sets.uuid, clusters.name, network_address_sets.project_name, network_address_sets.name, network_address_sets.object, network_address_sets.last_updated
FROM network_address_sets
  INNER JOIN clusters ON network_address_sets.cluster_id = clusters.id
WHERE true
%s
ORDER BY clusters.name, network_address_sets.name
`

	var whereClause []string
	var args []any

	if filter.Cluster != nil {
		whereClause = append(whereClause, ` AND clusters.name = :cluster_name`)
		args = append(args, sql.Named("cluster_name", filter.Cluster))
	}

	if filter.Project != nil {
		whereClause = append(whereClause, ` AND network_address_sets.project_name = :project`)
		args = append(args, sql.Named("project", filter.Project))
	}

	sqlStmtComplete := fmt.Sprintf(sqlStmt, strings.Join(whereClause, " "))

	rows, err := r.db.QueryContext(ctx, sqlStmtComplete, args...)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var networkAddressSets inventory.NetworkAddressSets
	for rows.Next() {
		var networkAddressSet inventory.NetworkAddressSet
		networkAddressSet, err = scanNetworkAddressSet(rows)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		networkAddressSets = append(networkAddressSets, networkAddressSet)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return networkAddressSets, nil
}

func (r networkAddressSet) GetAllUUIDsWithFilter(ctx context.Context, filter inventory.NetworkAddressSetFilter) ([]uuid.UUID, error) {
	const sqlStmt = `
SELECT network_address_sets.uuid
FROM network_address_sets
  INNER JOIN clusters ON network_address_sets.cluster_id = clusters.id
WHERE true
%s
ORDER BY network_address_sets.id
`

	var whereClause []string
	var args []any

	if filter.Cluster != nil {
		whereClause = append(whereClause, ` AND clusters.name = :cluster_name`)
		args = append(args, sql.Named("cluster_name", filter.Cluster))
	}

	if filter.Project != nil {
		whereClause = append(whereClause, ` AND network_address_sets.project_name = :project`)
		args = append(args, sql.Named("project", filter.Project))
	}

	sqlStmtComplete := fmt.Sprintf(sqlStmt, strings.Join(whereClause, " "))

	rows, err := r.db.QueryContext(ctx, sqlStmtComplete, args...)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		err := rows.Scan(&id)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		ids = append(ids, id)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return ids, nil
}

func (r networkAddressSet) GetByUUID(ctx context.Context, id uuid.UUID) (inventory.NetworkAddressSet, error) {
	const sqlStmt = `
SELECT
  network_address_sets.id, network_address_sets.uuid, clusters.name, network_address_sets.project_name, network_address_sets.name, network_address_sets.object, network_address_sets.last_updated
FROM
  network_address_sets
  INNER JOIN clusters ON network_address_sets.cluster_id = clusters.id
WHERE network_address_sets.uuid=:uuid;
`

	row := r.db.QueryRowContext(ctx, sqlStmt, sql.Named("uuid", id))
	if row.Err() != nil {
		return inventory.NetworkAddressSet{}, sqlite.MapErr(row.Err())
	}

	return scanNetworkAddressSet(row)
}

func (r networkAddressSet) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	const sqlStmt = `DELETE FROM network_address_sets WHERE uuid=:uuid;`

	result, err := r.db.ExecContext(ctx, sqlStmt, sql.Named("uuid", id))
	if err != nil {
		return sqlite.MapErr(err)
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return sqlite.MapErr(err)
	}

	if affectedRows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r networkAddressSet) DeleteByClusterName(ctx context.Context, cluster string) error {
	const sqlStmt = `
WITH _lookup AS (
  SELECT id as cluster_id from clusters where name = :cluster_name
)
DELETE FROM network_address_sets WHERE cluster_id=(SELECT cluster_id FROM _lookup);`

	result, err := r.db.ExecContext(ctx, sqlStmt, sql.Named("cluster_name", cluster))
	if err != nil {
		return sqlite.MapErr(err)
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return sqlite.MapErr(err)
	}

	if affectedRows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r networkAddressSet) UpdateByUUID(ctx context.Context, in inventory.NetworkAddressSet) (inventory.NetworkAddressSet, error) {
	const sqlStmt = `
WITH _lookup AS (
  SELECT id AS cluster_id FROM clusters WHERE clusters.name = :cluster_name
)
UPDATE network_address_sets SET uuid=:uuid, cluster_id=(SELECT cluster_id FROM _lookup), project_name=:project_name, name=:name, object=:object, last_updated=:last_updated
WHERE uuid=:uuid
RETURNING id, :uuid, :cluster_name, project_name, name, object, last_updated;
`

	marshaledObject, err := json.Marshal(in.Object)
	if err != nil {
		return inventory.NetworkAddressSet{}, err
	}

	row := r.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("uuid", in.UUID),
		sql.Named("cluster_name", in.Cluster),
		sql.Named("project_name", in.ProjectName),
		sql.Named("name", in.Name),
		sql.Named("object", marshaledObject),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return inventory.NetworkAddressSet{}, sqlite.MapErr(row.Err())
	}

	return scanNetworkAddressSet(row)
}

func scanNetworkAddressSet(row interface{ Scan(dest ...any) error }) (inventory.NetworkAddressSet, error) {
	var object []byte
	var networkAddressSet inventory.NetworkAddressSet

	err := row.Scan(
		&networkAddressSet.ID,
		&networkAddressSet.UUID,
		&networkAddressSet.Cluster,
		&networkAddressSet.ProjectName,
		&networkAddressSet.Name,
		&object,
		&networkAddressSet.LastUpdated,
	)
	if err != nil {
		return inventory.NetworkAddressSet{}, sqlite.MapErr(err)
	}

	err = json.Unmarshal(object, &networkAddressSet.Object)
	if err != nil {
		return inventory.NetworkAddressSet{}, err
	}

	return networkAddressSet, nil
}
