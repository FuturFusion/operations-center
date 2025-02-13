package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

type instance struct {
	db sqlite.DBTX
}

var _ inventory.InstanceRepo = &instance{}

func NewInstance(db sqlite.DBTX) *instance {
	return &instance{
		db: db,
	}
}

func (i instance) Create(ctx context.Context, in inventory.Instance) (inventory.Instance, error) {
	const sqlStmt = `
WITH _server AS (
  SELECT cluster_id FROM servers WHERE server_id = :server_id
)
INSERT INTO instances (server_id, project_name, name, object, last_updated)
VALUES(:server_id, :project_name, :name, :object, :last_updated)
RETURNING id, (SELECT cluster_id FROM _server) as cluster_id, server_id, project_name, name, object, last_updated;
`

	marshaledObject, err := json.Marshal(in.Object)
	if err != nil {
		return inventory.Instance{}, err
	}

	row := i.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("server_id", in.ServerID),
		sql.Named("project_name", in.ProjectName),
		sql.Named("name", in.Name),
		sql.Named("object", marshaledObject),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return inventory.Instance{}, sqlite.MapErr(row.Err())
	}

	return scanInstance(row)
}

func (i instance) GetAllIDs(ctx context.Context) ([]int, error) {
	const sqlStmt = `SELECT id FROM instances ORDER BY id`

	rows, err := i.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
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

func (i instance) GetByID(ctx context.Context, id int) (inventory.Instance, error) {
	const sqlStmt = `
SELECT
  instances.id, servers.cluster_id as cluster_id, instances.server_id, instances.project_name, instances.name, instances.object, instances.last_updated
FROM
  instances
  INNER JOIN servers ON instances.server_id = servers.id
WHERE instances.id=:id;
`

	row := i.db.QueryRowContext(ctx, sqlStmt, sql.Named("id", id))
	if row.Err() != nil {
		return inventory.Instance{}, sqlite.MapErr(row.Err())
	}

	return scanInstance(row)
}

func (i instance) DeleteByServerID(ctx context.Context, serverID int) error {
	const sqlStmt = `DELETE FROM instances WHERE server_id=:serverID;`

	result, err := i.db.ExecContext(ctx, sqlStmt, sql.Named("serverID", serverID))
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

func scanInstance(row interface{ Scan(dest ...any) error }) (inventory.Instance, error) {
	var object []byte
	var instance inventory.Instance

	err := row.Scan(
		&instance.ID,
		&instance.ClusterID,
		&instance.ServerID,
		&instance.ProjectName,
		&instance.Name,
		&object,
		&instance.LastUpdated,
	)
	if err != nil {
		return inventory.Instance{}, sqlite.MapErr(err)
	}

	err = json.Unmarshal(object, &instance.Object)
	if err != nil {
		return inventory.Instance{}, err
	}

	return instance, nil
}
