package sqlite

import (
	"context"
	"database/sql"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

type cluster struct {
	db sqlite.DBTX
}

var _ provisioning.ClusterRepo = &cluster{}

func NewCluster(db sqlite.DBTX) *cluster {
	return &cluster{
		db: db,
	}
}

func (c cluster) Create(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
	const sqlStmt = `
INSERT INTO clusters (name, connection_url, last_updated)
VALUES(:name, :connection_url, :last_updated)
RETURNING name, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("name", in.Name),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return provisioning.Cluster{}, sqlite.MapErr(row.Err())
	}

	return scanCluster(row)
}

func (c cluster) GetAll(ctx context.Context) (provisioning.Clusters, error) {
	const sqlStmt = `SELECT name, connection_url, last_updated FROM clusters;`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var clusters provisioning.Clusters
	for rows.Next() {
		cluster, err := scanCluster(rows)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		clusters = append(clusters, cluster)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return clusters, nil
}

func (c cluster) GetAllNames(ctx context.Context) ([]string, error) {
	const sqlStmt = `SELECT name FROM clusters ORDER BY id`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, sqlite.MapErr(err)
	}

	defer func() { _ = rows.Close() }()

	var clusterNames []string
	for rows.Next() {
		var clusterName string
		err := rows.Scan(&clusterName)
		if err != nil {
			return nil, sqlite.MapErr(err)
		}

		clusterNames = append(clusterNames, clusterName)
	}

	if rows.Err() != nil {
		return nil, sqlite.MapErr(rows.Err())
	}

	return clusterNames, nil
}

func (c cluster) GetByName(ctx context.Context, name string) (provisioning.Cluster, error) {
	const sqlStmt = `SELECT name, connection_url, last_updated FROM clusters WHERE name=:name;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("name", name))
	if row.Err() != nil {
		return provisioning.Cluster{}, sqlite.MapErr(row.Err())
	}

	return scanCluster(row)
}

func (c cluster) UpdateByName(ctx context.Context, name string, in provisioning.Cluster) (provisioning.Cluster, error) {
	const sqlStmt = `
UPDATE clusters SET connection_url=:connection_url, last_updated=:last_updated
WHERE name=:name
RETURNING name, connection_url, last_updated;
`

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("name", in.Name),
		sql.Named("connection_url", in.ConnectionURL),
		sql.Named("last_updated", in.LastUpdated),
	)
	if row.Err() != nil {
		return provisioning.Cluster{}, sqlite.MapErr(row.Err())
	}

	return scanCluster(row)
}

func (c cluster) DeleteByName(ctx context.Context, name string) error {
	const sqlStmt = `DELETE FROM clusters WHERE name=:name;`

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

func scanCluster(row interface{ Scan(dest ...any) error }) (provisioning.Cluster, error) {
	var cluster provisioning.Cluster

	err := row.Scan(
		&cluster.Name,
		&cluster.ConnectionURL,
		&cluster.LastUpdated,
	)
	if err != nil {
		return provisioning.Cluster{}, sqlite.MapErr(err)
	}

	return cluster, nil
}
