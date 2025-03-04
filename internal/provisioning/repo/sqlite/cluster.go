package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sqlite"
)

type cluster struct {
	db sqlite.DBTX
}

var _ provisioning.ClusterRepo = &cluster{}

func NewCluster(db sqlite.DBTX) (*cluster, error) {
	dbprepare, ok := db.(interface {
		Prepare(query string) (*sql.Stmt, error)
	})
	if !ok {
		return nil, fmt.Errorf("Provided db does not support prepare")
	}

	stmts, err := entities.PrepareStmts(dbprepare, false)
	if err != nil {
		return nil, fmt.Errorf("Failed to prepare statements: %w", err)
	}

	entities.PreparedStmts = stmts

	return &cluster{
		db: db,
	}, nil
}

func (c cluster) Create(ctx context.Context, in provisioning.Cluster) (provisioning.Cluster, error) {
	_, err := entities.CreateCluster(ctx, c.db, entities.Cluster{
		Name:            in.Name,
		ConnectionURL:   in.ConnectionURL,
		ServerHostnames: in.ServerHostnames,
		LastUpdated:     in.LastUpdated,
	})
	if err != nil {
		return provisioning.Cluster{}, err
	}

	return in, nil
}

func (c cluster) GetAll(ctx context.Context) (provisioning.Clusters, error) {
	dbClusters, err := entities.GetClusters(ctx, c.db)
	if err != nil {
		return nil, err
	}

	clusters := make(provisioning.Clusters, 0, len(dbClusters))
	for _, dbCluster := range dbClusters {
		clusters = append(clusters, provisioning.Cluster{
			Name:            dbCluster.Name,
			ConnectionURL:   dbCluster.ConnectionURL,
			ServerHostnames: dbCluster.ServerHostnames,
			LastUpdated:     dbCluster.LastUpdated,
		})
	}

	return clusters, nil
}

func (c cluster) GetAllNames(ctx context.Context) ([]string, error) {
	// TODO: fix overfetching, we don't need all the servers, we only need the names
	dbClusters, err := entities.GetClusters(ctx, c.db)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(dbClusters))
	for _, cluster := range dbClusters {
		names = append(names, cluster.Name)
	}

	return names, nil
}

func (c cluster) GetByName(ctx context.Context, name string) (provisioning.Cluster, error) {
	dbCluster, err := entities.GetCluster(ctx, c.db, name)
	if err != nil {
		return provisioning.Cluster{}, err
	}

	return provisioning.Cluster{
		Name:            dbCluster.Name,
		ConnectionURL:   dbCluster.ConnectionURL,
		ServerHostnames: dbCluster.ServerHostnames,
		LastUpdated:     dbCluster.LastUpdated,
	}, nil
}

func (c cluster) UpdateByName(ctx context.Context, name string, in provisioning.Cluster) (provisioning.Cluster, error) {
	err := entities.UpdateCluster(ctx, c.db, name, entities.Cluster{
		Name:            name,
		ConnectionURL:   in.ConnectionURL,
		ServerHostnames: in.ServerHostnames,
		LastUpdated:     in.LastUpdated,
	})
	in.Name = name
	return in, err
}

func (c cluster) Rename(ctx context.Context, name string, to string) error {
	return entities.RenameCluster(ctx, c.db, name, to)
}

func (c cluster) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteCluster(ctx, c.db, name)
}
