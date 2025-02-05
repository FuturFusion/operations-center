package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/mattn/go-sqlite3"

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
INSERT INTO clusters (name, server_hostnames)
VALUES(:name, :server_hostnames)
RETURNING id, name, server_hostnames;
`

	serverHostnames := strings.Join(in.ServerHostnames, ",")

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("name", in.Name),
		sql.Named("server_hostnames", serverHostnames),
	)
	if row.Err() != nil {
		return provisioning.Cluster{}, row.Err()
	}

	return scanCluster(row)
}

func (c cluster) GetAll(ctx context.Context) (provisioning.Clusters, error) {
	const sqlStmt = `SELECT id, name, server_hostnames FROM clusters;`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var clusters provisioning.Clusters
	for rows.Next() {
		cluster, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}

		clusters = append(clusters, cluster)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return clusters, nil
}

func (c cluster) GetAllNames(ctx context.Context) ([]string, error) {
	const sqlStmt = `SELECT name FROM clusters ORDER BY id`

	rows, err := c.db.QueryContext(ctx, sqlStmt)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var clusterNames []string
	for rows.Next() {
		var clusterName string
		err := rows.Scan(&clusterName)
		if err != nil {
			return nil, err
		}

		clusterNames = append(clusterNames, clusterName)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return clusterNames, nil
}

func (c cluster) GetByID(ctx context.Context, id int) (provisioning.Cluster, error) {
	const sqlStmt = `SELECT id, name, server_hostnames FROM clusters WHERE id=:id;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("id", id))
	if row.Err() != nil {
		return provisioning.Cluster{}, row.Err()
	}

	return scanCluster(row)
}

func (c cluster) GetByName(ctx context.Context, name string) (provisioning.Cluster, error) {
	const sqlStmt = `SELECT id, name, server_hostnames FROM clusters WHERE name=:name;`

	row := c.db.QueryRowContext(ctx, sqlStmt, sql.Named("name", name))
	if row.Err() != nil {
		return provisioning.Cluster{}, row.Err()
	}

	return scanCluster(row)
}

func (c cluster) UpdateByID(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
	const sqlStmt = `
UPDATE clusters SET server_hostnames=:server_hostnames
WHERE id=:id
RETURNING id, name, server_hostnames;
`

	serverHostnames := strings.Join(in.ServerHostnames, ",")

	row := c.db.QueryRowContext(ctx, sqlStmt,
		sql.Named("id", in.ID),
		sql.Named("server_hostnames", serverHostnames),
	)
	if row.Err() != nil {
		return provisioning.Cluster{}, row.Err()
	}

	return scanCluster(row)
}

func (c cluster) DeleteByID(ctx context.Context, id int) error {
	const sqlStmt = `DELETE FROM clusters WHERE id=:id;`

	result, err := c.db.ExecContext(ctx, sqlStmt, sql.Named("id", id))
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if sqliteErr.Code == sqlite3.ErrConstraint {
				return domain.ErrConstraintViolation
			}
		}

		return err
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affectedRows == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func scanCluster(row interface{ Scan(dest ...any) error }) (provisioning.Cluster, error) {
	var cluster provisioning.Cluster
	var serverNames string
	err := row.Scan(
		&cluster.ID,
		&cluster.Name,
		&serverNames,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return provisioning.Cluster{}, domain.ErrNotFound
		}

		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if sqliteErr.Code == sqlite3.ErrConstraint {
				return provisioning.Cluster{}, domain.ErrConstraintViolation
			}
		}

		return provisioning.Cluster{}, err
	}

	cluster.ServerHostnames = strings.Split(serverNames, ",")

	return cluster, nil
}
