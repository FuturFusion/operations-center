package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo"
)

type server struct {
	db repo.DBTX
}

var _ provisioning.ServerRepo = &server{}

func NewServer(db repo.DBTX) *server {
	return &server{
		db: db,
	}
}

func (c server) Create(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
	const sqlStmt = `
INSERT INTO servers (cluster_id, hostname, type, connection_url, last_updated)
VALUES(:cluster_id, :hostname, :type, :connection_url, :last_updated)
RETURNING id, cluster_id, hostname, type, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("cluster_id", in.ClusterID),
		sql.Named("hostname", in.Hostname),
		sql.Named("type", in.Type),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("last_updated", datetime(in.LastUpdated)),
	)
	if row.Err() != nil {
		return provisioning.Server{}, mapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) GetAll(ctx context.Context) (provisioning.Servers, error) {
	const sqlStmt = `SELECT id, cluster_id, hostname, type, connection_url, last_updated FROM servers;`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, mapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var servers provisioning.Servers
	for rows.Next() {
		server, err := scanServer(rows)
		if err != nil {
			return nil, mapErr(err)
		}

		servers = append(servers, server)
	}

	if rows.Err() != nil {
		return nil, mapErr(rows.Err())
	}

	return servers, nil
}

func (c server) GetAllHostnames(ctx context.Context) ([]string, error) {
	const sqlStmt = `SELECT hostname FROM servers ORDER BY id`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, mapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var serverHostnames []string
	for rows.Next() {
		var serverHostname string
		err := rows.Scan(&serverHostname)
		if err != nil {
			return nil, mapErr(err)
		}

		serverHostnames = append(serverHostnames, serverHostname)
	}

	if rows.Err() != nil {
		return nil, mapErr(rows.Err())
	}

	return serverHostnames, nil
}

func (c server) GetByID(ctx context.Context, id int) (provisioning.Server, error) {
	const sqlStmt = `SELECT id, cluster_id, hostname, type, connection_url, last_updated FROM servers WHERE id=:id;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("id", id))
	if row.Err() != nil {
		return provisioning.Server{}, mapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) GetByHostname(ctx context.Context, hostname string) (provisioning.Server, error) {
	const sqlStmt = `SELECT id, cluster_id, hostname, type, connection_url, last_updated FROM servers WHERE hostname=:hostname;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("hostname", hostname))
	if row.Err() != nil {
		return provisioning.Server{}, mapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) UpdateByID(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
	const sqlStmt = `
UPDATE servers SET cluster_id=:cluster_id, type=:type, connection_url=:connection_url, last_updated=:last_updated
WHERE id=:id
RETURNING id, cluster_id, hostname, type, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("id", in.ID),
		sql.Named("cluster_id", in.ClusterID),
		sql.Named("type", in.Type),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("last_updated", datetime(in.LastUpdated)),
	)
	if row.Err() != nil {
		return provisioning.Server{}, mapErr(row.Err())
	}

	return scanServer(row)
}

func (c server) DeleteByID(ctx context.Context, id int) error {
	const sqlStmt = `DELETE FROM servers WHERE id=:id;`

	result, err := c.db.ExecContext(ctx, sqlStmt, sql.Named("id", id))
	if err != nil {
		return mapErr(err)
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return mapErr(err)
	}

	if affectedRows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func scanServer(row interface{ Scan(dest ...any) error }) (provisioning.Server, error) {
	var server provisioning.Server
	var lastUpdated datetime

	err := row.Scan(
		&server.ID,
		&server.ClusterID,
		&server.Hostname,
		&server.Type,
		&server.ConnectionURL,
		&lastUpdated,
	)
	if err != nil {
		return provisioning.Server{}, mapErr(err)
	}

	server.LastUpdated = time.Time(lastUpdated)

	return server, nil
}
