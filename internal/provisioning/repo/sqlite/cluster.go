package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo"
)

type cluster struct {
	db repo.DBTX
}

var _ provisioning.ClusterRepo = &cluster{}

func NewCluster(db repo.DBTX) *cluster {
	return &cluster{
		db: db,
	}
}

func (c cluster) Create(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
	const sqlStmt = `
INSERT INTO clusters (name, connection_url, server_hostnames, last_updated)
VALUES(:name, :connection_url, :server_hostnames, :last_updated)
RETURNING id, name, connection_url, server_hostnames, last_updated;
`

	serverHostnames := strings.Join(in.ServerHostnames, ",")

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("name", in.Name),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("server_hostnames", serverHostnames),
		sql.Named("last_updated", datetime(in.LastUpdated)),
	)
	if row.Err() != nil {
		return provisioning.Cluster{}, mapErr(row.Err())
	}

	return scanCluster(row)
}

func (c cluster) GetAll(ctx context.Context) (provisioning.Clusters, error) {
	const sqlStmt = `SELECT id, name, connection_url, server_hostnames, last_updated FROM clusters;`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, mapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var clusters provisioning.Clusters
	for rows.Next() {
		cluster, err := scanCluster(rows)
		if err != nil {
			return nil, mapErr(err)
		}

		clusters = append(clusters, cluster)
	}

	if rows.Err() != nil {
		return nil, mapErr(rows.Err())
	}

	return clusters, nil
}

func (c cluster) GetAllNames(ctx context.Context) ([]string, error) {
	const sqlStmt = `SELECT name FROM clusters ORDER BY id`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, mapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var clusterNames []string
	for rows.Next() {
		var clusterName string
		err := rows.Scan(&clusterName)
		if err != nil {
			return nil, mapErr(err)
		}

		clusterNames = append(clusterNames, clusterName)
	}

	if rows.Err() != nil {
		return nil, mapErr(rows.Err())
	}

	return clusterNames, nil
}

func (c cluster) GetByID(ctx context.Context, id int) (provisioning.Cluster, error) {
	const sqlStmt = `SELECT id, name, connection_url, server_hostnames, last_updated FROM clusters WHERE id=:id;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("id", id))
	if row.Err() != nil {
		return provisioning.Cluster{}, mapErr(row.Err())
	}

	return scanCluster(row)
}

func (c cluster) GetByName(ctx context.Context, name string) (provisioning.Cluster, error) {
	const sqlStmt = `SELECT id, name, connection_url, server_hostnames, last_updated FROM clusters WHERE name=:name;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("name", name))
	if row.Err() != nil {
		return provisioning.Cluster{}, mapErr(row.Err())
	}

	return scanCluster(row)
}

func (c cluster) UpdateByID(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
	const sqlStmt = `
UPDATE clusters SET connection_url=:connection_url, server_hostnames=:server_hostnames, last_updated=:last_updated
WHERE id=:id
RETURNING id, name, connection_url, server_hostnames, last_updated;
`

	serverHostnames := strings.Join(in.ServerHostnames, ",")

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("id", in.ID),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("server_hostnames", serverHostnames),
		sql.Named("last_updated", datetime(in.LastUpdated)),
	)
	if row.Err() != nil {
		return provisioning.Cluster{}, mapErr(row.Err())
	}

	return scanCluster(row)
}

func (c cluster) DeleteByID(ctx context.Context, id int) error {
	const sqlStmt = `DELETE FROM clusters WHERE id=:id;`

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

func scanCluster(row interface{ Scan(dest ...any) error }) (provisioning.Cluster, error) {
	var cluster provisioning.Cluster
	var serverNames string
	var lastUpdated datetime

	err := row.Scan(
		&cluster.ID,
		&cluster.Name,
		&cluster.ConnectionURL,
		&serverNames,
		&lastUpdated,
	)
	if err != nil {
		return provisioning.Cluster{}, mapErr(err)
	}

	cluster.ServerHostnames = strings.Split(serverNames, ",")
	cluster.LastUpdated = time.Time(lastUpdated)

	return cluster, nil
}
