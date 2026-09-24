package sqlite

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"

	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// databaseFilename is the name of the database file used by dbdriver.Open.
const databaseFilename = "local.db"

// inventoryTables are omitted from a snapshot, they are synced again after a restore.
var inventoryTables = []string{
	"images",
	"instances",
	"networks",
	"network_acls",
	"network_address_sets",
	"network_forwards",
	"network_integrations",
	"network_load_balancers",
	"network_peers",
	"network_zones",
	"profiles",
	"projects",
	"storage_buckets",
	"storage_pools",
	"storage_volumes",
}

// imageTables are omitted from a snapshot without images, they are fetched again from the image sources.
var imageTables = []string{
	"incus_images",
}

type database struct {
	db dbdriver.DBTX
}

var _ system.DatabaseRepo = database{}

func NewDatabase(db dbdriver.DBTX) database {
	return database{
		db: db,
	}
}

func (d database) Snapshot(ctx context.Context, dir string, options system.BackupOptions) error {
	// VACUUM INTO creates a transactionally consistent copy of the live database.
	_, err := d.db.ExecContext(ctx, `VACUUM INTO ?`, filepath.Join(dir, databaseFilename))
	if err != nil {
		return fmt.Errorf("Failed to create database snapshot: %w", err)
	}

	snapshot, err := dbdriver.Open(dir)
	if err != nil {
		return err
	}

	defer func() {
		err := snapshot.Close()
		if err != nil {
			slog.WarnContext(ctx, "Failed to close database snapshot", slog.String("directory", dir), logger.Err(err))
		}
	}()

	_, err = snapshot.ExecContext(ctx, `PRAGMA foreign_keys=OFF`)
	if err != nil {
		return fmt.Errorf("Failed to disable foreign keys on database snapshot: %w", err)
	}

	tables := inventoryTables
	if !options.Images {
		tables = append(slices.Clone(tables), imageTables...)
	}

	for _, table := range tables {
		_, err = snapshot.ExecContext(ctx, `DELETE FROM `+table)
		if err != nil {
			return fmt.Errorf("Failed to remove table %q from database snapshot: %w", table, err)
		}
	}

	_, err = snapshot.ExecContext(ctx, `VACUUM`)
	if err != nil {
		return fmt.Errorf("Failed to compact database snapshot: %w", err)
	}

	return snapshot.Close()
}

func (d database) SchemaVersion(ctx context.Context, dir string) (current int, latest int, _ error) {
	db, err := dbdriver.Open(dir)
	if err != nil {
		return -1, -1, err
	}

	defer func() {
		err := db.Close()
		if err != nil {
			slog.WarnContext(ctx, "Failed to close database of backup", slog.String("directory", dir), logger.Err(err))
		}
	}()

	return dbschema.Version(ctx, db)
}
