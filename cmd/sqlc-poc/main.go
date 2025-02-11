package main

import (
	"context"
	"database/sql"
	_ "embed"
	"log"
	"reflect"
	"time"

	_ "github.com/mattn/go-sqlite3"

	model "github.com/FuturFusion/operations-center/cmd/sqlc-poc/db"
	"github.com/FuturFusion/operations-center/cmd/sqlc-poc/ptr"
)

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc generate

//go:embed db/schema/schema.sql
var ddl string

func run() error {
	ctx := context.Background()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return err
	}

	// create tables
	_, err = db.ExecContext(ctx, ddl)
	if err != nil {
		return err
	}

	queries := model.New(db)

	// list all clusters
	clusters, err := queries.ListClusters(ctx)
	if err != nil {
		return err
	}

	log.Println(clusters)

	// create an cluster
	insertedCluster, err := queries.CreateCluster(ctx, model.CreateClusterParams{
		Name:            "two",
		ConnectionUrl:   "http://localhost/",
		ServerHostnames: model.StringSlice{"srv10", "srv11"},
		LastUpdated:     time.Now().UTC().Truncate(0),
	})
	if err != nil {
		return err
	}

	log.Println(insertedCluster)

	// get the cluster we just inserted
	fetchedCluster, err := queries.GetCluster(ctx, insertedCluster.ID)
	if err != nil {
		return err
	}

	// prints true
	log.Println(reflect.DeepEqual(insertedCluster, fetchedCluster))

	// list all servers
	servers, err := queries.ListServers(ctx)
	if err != nil {
		return err
	}

	log.Println(servers)

	// list all storage_volumes
	storageVolumes, err := queries.ListStorageVolumes(ctx)
	if err != nil {
		return err
	}

	log.Println(storageVolumes)

	// Get filtered storage volumes with all filters provided.
	storageVolumes2, err := queries.ListStorageVolumesFiltered(ctx, model.ListStorageVolumesFilteredParams{
		ServerID:  ptr.To(int64(1)),
		ClusterID: ptr.To(int64(1)),
		ProjectID: ptr.To(int64(1)),
		Name:      ptr.To("one"),
	})
	if err != nil {
		return err
	}

	log.Println(storageVolumes2)

	// Get filtered storage volumes with only one filter provided.
	storageVolumes2, err = queries.ListStorageVolumesFiltered(ctx, model.ListStorageVolumesFilteredParams{
		ProjectID: ptr.To(int64(2)),
	})
	if err != nil {
		return err
	}

	log.Println(storageVolumes2)

	// Get filtered storage volumes with no filters.
	storageVolumes2, err = queries.ListStorageVolumesFiltered(ctx, model.ListStorageVolumesFilteredParams{})
	if err != nil {
		return err
	}

	log.Println(storageVolumes2)

	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}
