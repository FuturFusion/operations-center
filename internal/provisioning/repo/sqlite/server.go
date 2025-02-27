package sqlite

import (
	"context"
	"database/sql"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

type server struct {
	db sqlite.DBTX
}

var _ provisioning.ServerRepo = &server{}

func NewServer(db sqlite.DBTX) *server {
	return &server{
		db: db,
	}
}

func (c server) Create(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
	const sqlStmt = `
INSERT INTO servers (cluster_id, name, type, connection_url, last_updated)
VALUES(:cluster_id, :name, :type, :connection_url, :last_updated)
RETURNING id, cluster_id, name, type, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("cluster_id", in.ClusterID),
		sql.Named("name", in.Name),
		sql.Named("type", in.Type),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return provisioning.Server{}, sqlite.MapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) GetAll(ctx context.Context) (provisioning.Servers, error) {
	const sqlStmt = `SELECT id, cluster_id, name, type, connection_url, last_updated FROM servers;`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var servers provisioning.Servers
	for rows.Next() {
		server, err := scanServer(rows)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		servers = append(servers, server)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return servers, nil
}

func (c server) GetAllByClusterID(ctx context.Context, clusterID int) (provisioning.Servers, error) {
	const sqlStmt = `
SELECT
  id, cluster_id, name, type, connection_url, last_updated
FROM servers
WHERE
  cluster_id = :cluster_id;
`

	rows, err := c.db.QueryContext(ctx, sqlStmt,
		sql.Named("cluster_id", clusterID),
	)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var servers provisioning.Servers
	for rows.Next() {
		server, err := scanServer(rows)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		servers = append(servers, server)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return servers, nil
}

func (c server) GetAllNames(ctx context.Context) ([]string, error) {
	const sqlStmt = `SELECT name FROM servers ORDER BY id`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var serverNames []string
	for rows.Next() {
		var serverName string
		err := rows.Scan(&serverName)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		serverNames = append(serverNames, serverName)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return serverNames, nil
}

func (c server) GetByID(ctx context.Context, id int) (provisioning.Server, error) {
	const sqlStmt = `SELECT id, cluster_id, name, type, connection_url, last_updated FROM servers WHERE id=:id;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("id", id))
	if row.Err() != nil {
		return provisioning.Server{}, sqlite.MapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) GetByName(ctx context.Context, name string) (provisioning.Server, error) {
	const sqlStmt = `SELECT id, cluster_id, name, type, connection_url, last_updated FROM servers WHERE name=:name;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("name", name))
	if row.Err() != nil {
		return provisioning.Server{}, sqlite.MapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) UpdateByID(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
	const sqlStmt = `
UPDATE servers SET cluster_id=:cluster_id, type=:type, connection_url=:connection_url, last_updated=:last_updated
WHERE id=:id
RETURNING id, cluster_id, name, type, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("id", in.ID),
		sql.Named("cluster_id", in.ClusterID),
		sql.Named("type", in.Type),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return provisioning.Server{}, sqlite.MapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) DeleteByID(ctx context.Context, id int) error {
	const sqlStmt = `DELETE FROM servers WHERE id=:id;`

	result, err := c.db.ExecContext(ctx, sqlStmt, sql.Named("id", id))
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

func scanServer(row interface{ Scan(dest ...any) error }) (provisioning.Server, error) {
	var server provisioning.Server

	err := row.Scan(
		&server.ID,
		&server.ClusterID,
		&server.Name,
		&server.Type,
		&server.ConnectionURL,
		&server.LastUpdated,
	)
	if err != nil {
		return provisioning.Server{}, sqlite.MapErr(err)
	}

	return server, nil
}
