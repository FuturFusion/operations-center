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
WITH _lookup AS (
  SELECT id AS cluster_id FROM clusters WHERE name = :cluster_name
)
INSERT INTO servers (cluster_id, name, type, connection_url, last_updated)
VALUES((SELECT cluster_id FROM _lookup), :name, :type, :connection_url, :last_updated)
RETURNING :cluster_name, name, type, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("cluster_name", in.Cluster),
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
	const sqlStmt = `
SELECT
  clusters.name AS cluster_name, servers.name, servers.type, servers.connection_url, servers.last_updated
FROM
  servers
  LEFT JOIN clusters ON servers.cluster_id = clusters.id;
`

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
  clusters.name AS cluster_name, servers.name, servers.type, servers.connection_url, servers.last_updated
FROM
  servers
  LEFT JOIN clusters ON servers.cluster_id = clusters.id
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
	const sqlStmt = `SELECT name FROM servers ORDER BY id;`

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

func (c server) GetByName(ctx context.Context, name string) (provisioning.Server, error) {
	const sqlStmt = `
SELECT
  clusters.name, servers.name, servers.type, servers.connection_url, servers.last_updated
FROM
  servers
  LEFT JOIN clusters ON servers.cluster_id = clusters.id
WHERE servers.name=:name;
`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("name", name))
	if row.Err() != nil {
		return provisioning.Server{}, sqlite.MapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) UpdateByName(ctx context.Context, name string, in provisioning.Server) (provisioning.Server, error) {
	const sqlStmt = `
WITH _lookup AS (
  SELECT id AS cluster_id, name AS cluster_name FROM clusters WHERE name = :cluster_name
)
UPDATE servers SET cluster_id=(SELECT cluster_id FROM _lookup), type=:type, connection_url=:connection_url, last_updated=:last_updated
WHERE name=:name
RETURNING :cluster_name, name, type, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("name", in.Name),
		sql.Named("cluster_name", in.Cluster),
		sql.Named("type", in.Type),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return provisioning.Server{}, sqlite.MapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) DeleteByName(ctx context.Context, name string) error {
	const sqlStmt = `DELETE FROM servers WHERE name=:name;`

	result, err := c.db.ExecContext(ctx, sqlStmt, sql.Named("name", name))
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

	var clusterName *string

	err := row.Scan(
		&clusterName,
		&server.Name,
		&server.Type,
		&server.ConnectionURL,
		&server.LastUpdated,
	)
	if err != nil {
		return provisioning.Server{}, sqlite.MapErr(err)
	}

	if clusterName != nil {
		server.Cluster = *clusterName
	}

	return server, nil
}
