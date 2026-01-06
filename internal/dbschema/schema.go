package dbschema

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FuturFusion/operations-center/internal/file"
)

// Ensure applies all relevant schema updates to the local database.
//
// Return the initial schema version found before starting the update, along
// with any error occurred.
func Ensure(ctx context.Context, db *sql.DB, dir string) (int, error) {
	backupDone := false

	schema := newFromMap(updates)
	schema.fresh(freshSchema)
	schema.hook(func(ctx context.Context, version int, tx *sql.Tx) error {
		if !backupDone {
			slog.InfoContext(ctx, `Updating the database schema. Backup made as "local.db.bak"`)
			path := filepath.Join(dir, "local.db")
			err := file.Copy(path, path+".bak")
			if err != nil {
				return err
			}

			backupDone = true
		}

		if version == -1 {
			slog.DebugContext(ctx, "Running pre-update queries from file for local DB schema")
		} else {
			slog.DebugContext(ctx, "Updating DB schema", slog.Int("from_version", version), slog.Int("to_version", version+1))
		}

		return nil
	})

	return schema.ensure(ctx, db)
}

// schema captures the schema of a database in terms of a series of ordered
// updates.
type schema struct {
	updates   []update // Ordered series of updates making up the schema
	hookFunc  hook     // Optional hook to execute whenever a update gets applied
	freshStmt string   // Optional SQL statement used to create schema from scratch
}

// update applies a specific schema change to a database, and returns an error
// if anything goes wrong.
type update func(context.Context, *sql.Tx) error

// hook is a callback that gets fired when a update gets applied.
type hook func(context.Context, int, *sql.Tx) error

// newFromMap creates a new schema Schema with the updates specified in the
// given map. The keys of the map are schema versions that when upgraded will
// trigger the associated Update value. It's required that the minimum key in
// the map is 1, and if key N is present then N-1 is present too, with N>1
// (i.e. there are no missing versions).
func newFromMap(versionsToUpdates map[int]update) *schema {
	// Collect all version keys.
	versions := []int{}
	for version := range versionsToUpdates {
		versions = append(versions, version)
	}

	// Sort the versions,
	sort.Ints(versions)

	// Build the updates slice.
	updates := []update{}
	for i, version := range versions {
		// Assert that we start from 1 and there are no gaps.
		if version != i+1 {
			panic(fmt.Sprintf("updates map misses version %d", i+1))
		}

		updates = append(updates, versionsToUpdates[version])
	}

	return &schema{
		updates: updates,
	}
}

// hook instructs the schema to invoke the given function whenever a update is
// about to be applied. The function gets passed the update version number and
// the running transaction, and if it returns an error it will cause the schema
// transaction to be rolled back. Any previously installed hook will be
// replaced.
func (s *schema) hook(hook hook) {
	s.hookFunc = hook
}

// fresh sets a statement that will be used to create the schema from scratch
// when bootstraping an empty database. It should be a "flattening" of the
// available updates, generated using the Dump() method. If not given, all
// patches will be applied in order.
func (s *schema) fresh(statement string) {
	s.freshStmt = statement
}

// ensure makes sure that the actual schema in the given database matches the
// one defined by our updates.
//
// All updates are applied transactionally. In case any error occurs the
// transaction will be rolled back and the database will remain unchanged.
//
// A update will be applied only if it hasn't been before (currently applied
// updates are tracked in the a 'shema' table, which gets automatically
// created).
//
// If no error occurs, the integer returned by this method is the
// initial version that the schema has been upgraded from.
func (s *schema) ensure(ctx context.Context, db *sql.DB) (int, error) {
	var current int

	err := transaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error

		err = ensureSchemaTableExists(ctx, tx)
		if err != nil {
			return err
		}

		current, err = queryCurrentVersion(ctx, tx)
		if err != nil {
			return err
		}

		// When creating the schema from scratch, use the fresh dump if
		// available. Otherwise just apply all relevant updates.
		if current == 0 && s.freshStmt != "" {
			_, err = tx.Exec(s.freshStmt)
			if err != nil {
				return fmt.Errorf("cannot apply fresh schema: %w", err)
			}

			return nil
		}

		err = ensureUpdatesAreApplied(ctx, tx, current, s.updates, s.hookFunc)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return -1, err
	}

	return current, nil
}

// transaction executes the given function within a database transaction with a 30s context timeout.
func transaction(ctx context.Context, db *sql.DB, f func(context.Context, *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		// If there is a leftover transaction let's try to rollback,
		// we'll then retry again.
		if strings.Contains(err.Error(), "cannot start a transaction within a transaction") {
			_, _ = db.Exec("ROLLBACK")
		}

		return fmt.Errorf("Failed to begin transaction: %w", err)
	}

	err = f(ctx, tx)
	if err != nil {
		return rollback(tx, err)
	}

	err = tx.Commit()
	if err == sql.ErrTxDone {
		err = nil // Ignore duplicate commits/rollbacks
	}

	return err
}

// rollback a transaction after the given error occurred. If the rollback
// succeeds the given error is returned, otherwise a new error that wraps it
// gets generated and returned.
func rollback(tx *sql.Tx, reason error) error {
	err := tx.Rollback()
	if err != nil {
		return fmt.Errorf("Failed to rollback transaction after reason: %w, error: %v", reason, err)
	}

	return reason
}

// Ensure that the schema exists.
func ensureSchemaTableExists(ctx context.Context, tx *sql.Tx) error {
	exists, err := doesSchemaTableExist(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to check if schema table is there: %w", err)
	}

	if !exists {
		err := createSchemaTable(tx)
		if err != nil {
			return fmt.Errorf("failed to create schema table: %w", err)
		}
	}

	return nil
}

// Return the highest update version currently applied. Zero means that no
// updates have been applied yet.
func queryCurrentVersion(ctx context.Context, tx *sql.Tx) (int, error) {
	versions, err := selectSchemaVersions(ctx, tx)
	if err != nil {
		return -1, fmt.Errorf("failed to fetch update versions: %w", err)
	}

	current := 0
	if len(versions) > 0 {
		err = checkSchemaVersionsHaveNoHoles(versions)
		if err != nil {
			return -1, err
		}

		current = versions[len(versions)-1] // Highest recorded version
	}

	return current, nil
}

// Apply any pending update that was not yet applied.
func ensureUpdatesAreApplied(ctx context.Context, tx *sql.Tx, current int, updates []update, hook hook) error {
	if current > len(updates) {
		return fmt.Errorf(
			"schema version '%d' is more recent than expected '%d'",
			current, len(updates))
	}

	// If there are no updates, there's nothing to do.
	if len(updates) == 0 {
		return nil
	}

	// Apply missing updates.
	for _, update := range updates[current:] {
		if hook != nil {
			err := hook(ctx, current, tx)
			if err != nil {
				return fmt.Errorf(
					"failed to execute hook (version %d): %v", current, err)
			}
		}

		err := update(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to apply update %d: %w", current, err)
		}

		current++

		err = insertSchemaVersion(tx, current)
		if err != nil {
			return fmt.Errorf("failed to insert version %d", current)
		}
	}

	return nil
}

// Check that the given list of update version numbers doesn't have "holes",
// that is each version equal the preceding version plus 1.
func checkSchemaVersionsHaveNoHoles(versions []int) error {
	// Ensure that there are no "holes" in the recorded versions.
	for i := range versions[:len(versions)-1] {
		if versions[i+1] != versions[i]+1 {
			return fmt.Errorf("Missing updates: %d to %d", versions[i], versions[i+1])
		}
	}

	return nil
}
