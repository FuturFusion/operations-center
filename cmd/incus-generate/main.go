package main

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/FuturFusion/operations-center/cmd/incus-generate/entities"
	"github.com/FuturFusion/operations-center/cmd/incus-generate/query"
	"github.com/FuturFusion/operations-center/cmd/sqlc-poc/ptr"
	_ "github.com/mattn/go-sqlite3"
)

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc generate

//go:embed schema.sql
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

	stmts, err := entities.PrepareStmts(db, false)
	if err != nil {
		return fmt.Errorf("Failed to prepare statements: %w", err)
	}

	entities.PreparedStmts = stmts

	return query.Transaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		clusters, err := entities.GetClusters(ctx, tx)
		if err != nil {
			return err
		}

		log.Println(clusters)

		// create a cluster
		_, err = entities.CreateCluster(ctx, tx, entities.Cluster{
			Name:            "two",
			ConnectionURL:   "http://localhost/",
			ServerHostnames: entities.StringSlice{"srv10", "srv11"},
			LastUpdated:     time.Now().UTC().Truncate(0),
		})
		if err != nil {
			return err
		}

		insertedCluster, err := entities.GetCluster(ctx, tx, "two")
		if err != nil {
			return err
		}

		log.Println(insertedCluster)

		// This is just here to match sqlc's output: incus-generate does not yet support RETURNING
		log.Println(true)

		// list all servers
		servers, err := entities.GetServers(ctx, tx)
		if err != nil {
			return err
		}

		log.Println(servers)

		// list all storage_volumes
		storageVolumes, err := entities.GetStorageVolumes(ctx, tx)
		if err != nil {
			return err
		}

		log.Println(storageVolumes)

		// Get filtered storage volumes with all filters provided.
		storageVolumes2, err := entities.GetStorageVolumes(ctx, tx, entities.StorageVolumeFilter{
			ServerID:  ptr.To(int64(1)),
			ProjectID: ptr.To(int64(1)),
			Name:      ptr.To("one"),
		})

		if err != nil {
			return err
		}

		log.Println(storageVolumes2)

		// Get filtered storage volumes with only one filter provided.
		storageVolumes2, err = entities.GetStorageVolumes(ctx, tx, entities.StorageVolumeFilter{
			ProjectID: ptr.To(int64(2)),
		})

		if err != nil {
			return err
		}

		log.Println(storageVolumes2)

		// This is just here to match sqlc's output: incus-generate rejects empty filters.
		log.Println(storageVolumes)

		return nil
	})
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}
