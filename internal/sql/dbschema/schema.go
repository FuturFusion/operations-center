package dbschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	clusterTx "github.com/cowsql/go-cowsql/cluster/db/transaction"
	"github.com/lxc/incus/v7/shared/osarch"
	"github.com/lxc/incus/v7/shared/revert"
	"github.com/lxc/incus/v7/shared/util"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/sql/patches"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite/query"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/maps"
)

// EnsureGlobal applies all relevant schema updates to the cluster database.
//
// Before actually doing anything, this function will make sure that all nodes
// in the cluster have a schema version and a number of API extensions that
// match our one. If it's not the case, we either return an error (if some
// nodes have version greater than us and we need to be upgraded), or return
// false and no error (if some nodes have a lower version, and we need to wait
// till they get upgraded and restarted).
func EnsureGlobal(ctx context.Context, db *sql.DB, address, dir string) (bool, int, error) {
	var someNodesAreBehind, backupDone bool

	schema := newFromMap(updates)
	schema.fresh(freshSchema)
	schema.file(filepath.Join(dir, "patch.global.sql"))
	schema.migrateFunc = migrateFromStandalone
	schema.hook(func(ctx context.Context, schemaVersion int, tx *sql.Tx) error {
		// Check if we're clustered
		clustered := true

		n, err := selectUnclusteredNodesCount(ctx, tx)
		if err != nil {
			return fmt.Errorf("Failed to fetch standalone member count: %w", err)
		}

		if n > 1 {
			// This should never happen, since we only add cluster members with valid addresses.
			return errors.New("Found more than one cluster member with a standalone address (0.0.0.0)")
		} else if n == 1 {
			clustered = false
		}

		// If we're not clustered, backup the local cluster database directory
		// before performing any schema change. This makes sense only in the
		// non-clustered case, because otherwise the directory would be
		// re-populated by replication.
		if !clustered && !backupDone {
			slog.InfoContext(ctx, `Updating the database schema. Backup made as "global.bak"`)
			err := DirCopy(
				filepath.Join(dir, "global"),
				filepath.Join(dir, "global.bak"),
			)
			if err != nil {
				return fmt.Errorf("Failed to backup global database: %w", err)
			}

			backupDone = true
		}

		if schemaVersion == -1 {
			slog.DebugContext(ctx, "Running pre-update queries from file for global DB schema")
		} else {
			slog.DebugContext(ctx, "Updating global DB schema", slog.Int("from", schemaVersion), slog.Int("to", schemaVersion+1))
		}

		return nil
	})

	schema.check(func(ctx context.Context, current int, tx *sql.Tx) error {
		// If we're bootstrapping a fresh schema, skip any check, since
		// it's safe to assume we are the only node.
		if current == 0 {
			return nil
		}

		// Check if we're clustered
		n, err := selectUnclusteredNodesCount(ctx, tx)
		if err != nil {
			return fmt.Errorf("Failed to fetch standalone member count: %w", err)
		}

		if n > 1 {
			// This should never happen, since we only add nodes with valid addresses.
			return errors.New("Found more than one cluster member with a standalone address (0.0.0.0)")
		} else if n == 1 {
			address = "0.0.0.0" // We're not clustered
		}

		if address != "" {
			// Update the schema and api_extension columns of ourselves.
			err = updateNodeVersion(tx, address, 0)
			if err != nil {
				return fmt.Errorf("Failed to update cluster member version info: %w", err)
			}
		}

		err = checkClusterIsUpgradable(ctx, tx, [2]int{len(updates), 0})
		if errors.Is(err, errSomeNodesAreBehind) {
			someNodesAreBehind = true
			return ErrGracefulAbort
		}

		return err
	})

	// Ensure the nodes table exists before checking the schema, retrying while members starting at once lock the database.
	var nodesCount int
	err := clusterTx.Retry(ctx, schemaRetries, func(ctx context.Context) error {
		return transaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
			err := createMembershipTables(ctx, tx)
			if err != nil {
				return fmt.Errorf("Failed to ensure 'cluster_members' table exists: %w", err)
			}

			nodesCount, err = query.Count(ctx, tx, "cluster_members", "")
			if err != nil {
				return fmt.Errorf("Failed to get 'cluster_members' count: %w", err)
			}

			return nil
		})
	})
	if err != nil {
		return false, -1, err
	}

	var initial, current int
	err = clusterTx.Retry(ctx, schemaRetries, func(_ context.Context) error {
		var err error
		initial, current, err = schema.ensure(ctx, db)
		return err
	})
	if someNodesAreBehind {
		return false, initial, nil
	}

	if err != nil {
		return false, initial, err
	}

	// When creating a database from scratch, insert an entry for node
	// 1. This is needed for referential integrity with other tables. Also,
	// create a default profile.
	if initial == 0 || nodesCount == 0 {
		arch, err := osarch.ArchitectureGetLocalID()
		if err != nil {
			return false, initial, err
		}

		err = transaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
			stmt := `INSERT INTO cluster_members(id, name, address, schema, api_extensions, description, architecture, failure_domain, config) VALUES(1, 'none', '0.0.0.0', ?, ?, ?, ?, '{}', '{}')`
			_, err := tx.ExecContext(ctx, stmt, SchemaVersion, 0, "", arch)
			if err != nil {
				return err
			}

			// TODO: Cluster groups?

			return nil
		})
		if err != nil {
			return false, initial, err
		}
	}

	return true, current, nil
}

// schemaRetries limits the attempts to update the schema of a locked database.
const schemaRetries = 250

// EnsureLocal applies all relevant schema updates to the local database.
//
// Return the initial schema version found before starting the update, along
// with any error occurred.
func EnsureLocal(ctx context.Context, db *sql.DB, dir string, updates map[int]update, freshSchema string) (int, error) {
	backupDone := false

	schema := newFromMap(updates)
	schema.fresh(freshSchema)
	schema.file(filepath.Join(dir, "patch.local.sql"))
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

	initial, _, err := schema.ensure(ctx, db)
	if err != nil {
		return -1, err
	}

	return initial, nil
}

// schema captures the schema of a database in terms of a series of ordered
// updates.
type schema struct {
	updates   []update // Ordered series of updates making up the schema
	hookFunc  hook     // Optional hook to execute whenever a update gets applied
	checkFunc hook     // Optional callback invoked before doing any update
	sqlPath   string   // Optional path to a file containing extra queries to run
	// sqlApplied reports whether the queries of sqlPath are part of the transaction.
	sqlApplied bool
	freshStmt  string // Optional SQL statement used to create schema from scratch

	// migrateFunc migrates data from elsewhere before anything else and returns the path of the imported database.
	migrateFunc func(ctx context.Context, tx *sql.Tx) (string, error)
	// migratedPath is the database imported by migrateFunc in the transaction.
	migratedPath string
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
	versions := make([]int, 0, len(versionsToUpdates))
	for version := range versionsToUpdates {
		versions = append(versions, version)
	}

	// Sort the versions,
	sort.Ints(versions)

	// Build the updates slice.
	updates := make([]update, 0, len(versions))
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

// Check instructs the schema to invoke the given function whenever Ensure is
// invoked, before applying any due update. It can be used for aborting the
// operation.
func (s *schema) check(check hook) {
	s.checkFunc = check
}

// File extra queries from a file. If the file is exists, all SQL queries in it
// will be executed transactionally at the very start of Ensure(), before
// anything else is done.
//
// If a schema hook was set with Hook(), it will be run before running the
// queries in the file and it will be passed a patch version equals to -1.
func (s *schema) file(path string) {
	s.sqlPath = path
}

// fresh sets a statement that will be used to create the schema from scratch
// when bootstraping an empty database. It should be a "flattening" of the
// available updates, generated using the Dump() method. If not given, all
// patches will be applied in order.
func (s *schema) fresh(statement string) {
	s.freshStmt = statement
}

// Read the given file (if it exists) and executes all queries it contains.
func (s *schema) applyPatches(ctx context.Context, tx *sql.Tx) error {
	if s.sqlPath == "" || !util.PathExists(s.sqlPath) {
		return nil
	}

	bytes, err := os.ReadFile(s.sqlPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	exists, err := doesSchemaTableExist(ctx, tx)
	if err != nil {
		return fmt.Errorf("Failed to check if schema table exists: %w", err)
	}

	if s.hookFunc != nil && exists {
		err := s.hookFunc(ctx, -1, tx)
		if err != nil {
			return fmt.Errorf("failed to execute hook: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx, string(bytes))
	if err != nil {
		return err
	}

	s.sqlApplied = true

	return nil
}

// removePatches removes the file of queries once they are committed.
func (s *schema) removePatches() error {
	if !s.sqlApplied {
		return nil
	}

	err := os.Remove(s.sqlPath)
	if err != nil {
		return fmt.Errorf("failed to remove file: %w", err)
	}

	return nil
}

// migrateFromStandalone dumps a local.db file into cowsql if there is no schema table yet.
func migrateFromStandalone(ctx context.Context, tx *sql.Tx) (string, error) {
	exists, err := doesSchemaTableExist(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("failed to check if schema table exists: %w", err)
	}

	if !exists {
		env := environment.New(config.ApplicationName, config.ApplicationEnvPrefix)
		_, err = os.Stat(filepath.Join(env.VarDir(), "local.db"))
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("Failed to check for existing local database: %w", err)
		}

		if err != nil {
			return "", nil
		}

		db, err := dbdriver.Open(filepath.Join(env.VarDir()))
		if err != nil {
			return "", err
		}

		defer func() { _ = db.Close() }()

		dump, err := dumpStatements(ctx, db, false)
		if err != nil {
			return "", err
		}

		_, err = tx.ExecContext(ctx, dump)
		if err != nil {
			return "", err
		}

		// Only a database with a schema is imported, the sqlite global database of the tests has none.
		imported, err := doesSchemaTableExist(ctx, tx)
		if err != nil {
			return "", fmt.Errorf("failed to check if schema table exists: %w", err)
		}

		if imported {
			return filepath.Join(env.VarDir(), "local.db"), nil
		}
	}

	return "", nil
}

// retireMigrated renames the committed import, so an empty global database does not import it again.
func (s *schema) retireMigrated() error {
	if s.migratedPath == "" {
		return nil
	}

	err := os.Rename(s.migratedPath, s.migratedPath+".migrated")
	if err != nil {
		return fmt.Errorf("Failed to rename the imported standalone database: %w", err)
	}

	return nil
}

// ensure makes sure that the actual schema in the given database matches the
// one defined by our updates.
//
// All updates are applied transactionally. In case any error occurs the
// transaction will be rolled back and the database will remain unchanged.
//
// A update will be applied only if it hasn't been before (currently applied
// updates are tracked in the a 'schema' table, which gets automatically
// created).
//
// If no error occurs, the integer returned by this method is the
// initial version that the schema has been upgraded from.
func (s *schema) ensure(ctx context.Context, db *sql.DB) (int, int, error) {
	var initial, current int
	var aborted bool

	// Disable foreign keys before performing a schema update so references aren't cascade deleted.
	_, err := db.Exec("PRAGMA foreign_keys=OFF; PRAGMA legacy_alter_table=ON")
	if err != nil {
		return -1, -1, err
	}

	err = transaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		if s.migrateFunc != nil {
			var err error
			s.migratedPath, err = s.migrateFunc(ctx, tx)
			if err != nil {
				return fmt.Errorf("Failed to migrate from standalone database: %w", err)
			}
		}

		err := s.applyPatches(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to execute queries from %s: %w", s.sqlPath, err)
		}

		err = ensureSchemaTableExists(ctx, tx)
		if err != nil {
			return err
		}

		initial, err = queryCurrentVersion(ctx, tx)
		if err != nil {
			return err
		}

		if s.checkFunc != nil {
			err := s.checkFunc(ctx, initial, tx)
			if errors.Is(err, ErrGracefulAbort) {
				// Abort the update gracefully, committing what
				// we've done so far.
				aborted = true
				return nil
			}

			if err != nil {
				return err
			}
		}

		// When creating the schema from scratch, use the fresh dump if
		// available. Otherwise just apply all relevant updates.
		if initial == 0 && s.freshStmt != "" {
			_, err = tx.Exec(s.freshStmt)
			if err != nil {
				return fmt.Errorf("cannot apply fresh schema: %w", err)
			}

			current, err = queryCurrentVersion(ctx, tx)
			if err != nil {
				return err
			}

			// Mark all patches as applied, as there is no state to update.
			for _, name := range patches.List() {
				err = patches.MarkApplied(ctx, tx, name)
				if err != nil {
					return err
				}
			}

			return nil
		}

		current, err = ensureUpdatesAreApplied(ctx, tx, initial, s.updates, s.hookFunc)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return -1, -1, err
	}

	err = s.removePatches()
	if err != nil {
		return -1, -1, err
	}

	err = s.retireMigrated()
	if err != nil {
		return -1, -1, err
	}

	// Re-enable foreign keys before completing.
	_, err = db.Exec("PRAGMA foreign_keys=ON; PRAGMA legacy_alter_table=OFF")
	if err != nil {
		return -1, -1, err
	}

	if aborted {
		return initial, current, ErrGracefulAbort
	}

	return initial, current, nil
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
func ensureUpdatesAreApplied(ctx context.Context, tx *sql.Tx, current int, updates []update, hook hook) (int, error) {
	if current > len(updates) {
		return -1, fmt.Errorf(
			"schema version '%d' is more recent than expected '%d'",
			current, len(updates),
		)
	}

	// If there are no updates, there's nothing to do.
	if len(updates) == 0 {
		return -1, nil
	}

	// Apply missing updates.
	for _, update := range updates[current:] {
		if hook != nil {
			err := hook(ctx, current, tx)
			if err != nil {
				return -1, fmt.Errorf(
					"failed to execute hook (version %d): %v", current, err,
				)
			}
		}

		preUpdateCounts, err := getTableRowCounts(ctx, tx)
		if err != nil {
			return -1, fmt.Errorf("failed to get pre update table row counts: %w", err)
		}

		err = update(ctx, tx)
		if err != nil {
			return -1, fmt.Errorf("failed to apply update %d: %w", current, err)
		}

		postUpdateCounts, err := getTableRowCounts(ctx, tx)
		if err != nil {
			return -1, fmt.Errorf("failed to get post update table row counts: %w", err)
		}

		err = ensureDBRows(preUpdateCounts, postUpdateCounts)
		if err != nil {
			return -1, fmt.Errorf("DB update caused unexpected row decrease: %w", err)
		}

		current++

		err = insertSchemaVersion(tx, current)
		if err != nil {
			return -1, fmt.Errorf("failed to insert version %d", current)
		}
	}

	return current, nil
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

var rowCountIgnoredTables = map[string]struct{}{
	"images":                 {},
	"instances":              {},
	"networks":               {},
	"network_acls":           {},
	"network_address_sets":   {},
	"network_forwards":       {},
	"network_integrations":   {},
	"network_load_balancers": {},
	"network_peers":          {},
	"network_zones":          {},
	"profiles":               {},
	"projects":               {},
	"storage_buckets":        {},
	"storage_pools":          {},
	"storage_volumes":        {},
}

func getTableRowCounts(ctx context.Context, tx *sql.Tx) (tableCounts map[string]int, err error) {
	tableCounts = make(map[string]int, 20)

	rows, err := tx.QueryContext(ctx, `SELECT name from sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%';`)
	if err != nil {
		return nil, fmt.Errorf("Failed to query tables: %w", err)
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		err = errors.Join(err, rows.Close())
	})

	var tables []string

	for rows.Next() {
		var tableName string
		err = rows.Scan(&tableName)
		if err != nil {
			return nil, fmt.Errorf("Failed to scan table name: %w", err)
		}

		_, ok := rowCountIgnoredTables[tableName]
		if ok {
			continue
		}

		tables = append(tables, tableName)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("Failed to read table names rows: %w", rows.Err())
	}

	reverter.Success()

	err = rows.Close()
	if err != nil {
		return nil, fmt.Errorf("Failed to close rows: %w", err)
	}

	for _, tableName := range tables {
		row := tx.QueryRowContext(ctx, `SELECT count(1) FROM `+tableName)
		if row.Err() != nil {
			return nil, fmt.Errorf("Failed to query row counts for %q: %w", tableName, row.Err())
		}

		var count int
		err = row.Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("Failed to scan count: %w", err)
		}

		tableCounts[tableName] = count
	}

	return tableCounts, nil
}

func ensureDBRows(before map[string]int, after map[string]int) error {
	for tableName, beforeCount := range maps.OrderedByKey(before) {
		afterCount, ok := after[tableName]
		if !ok {
			return fmt.Errorf("no after counts for table %q, before count is %d", tableName, beforeCount)
		}

		if beforeCount > afterCount {
			return fmt.Errorf("number of rows for table %q decreased from %d to %d", tableName, beforeCount, afterCount)
		}
	}

	return nil
}

// Version returns the current and the latest known schema version of db.
func Version(ctx context.Context, db *sql.DB) (current int, latest int, _ error) {
	err := transaction(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		exists, err := doesSchemaTableExist(ctx, tx)
		if err != nil {
			return fmt.Errorf("Failed to check if schema table is there: %w", err)
		}

		if !exists {
			return nil
		}

		current, err = queryCurrentVersion(ctx, tx)
		return err
	})
	if err != nil {
		return -1, -1, err
	}

	return current, len(updates), nil
}
