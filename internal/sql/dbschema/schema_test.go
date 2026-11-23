package dbschema_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func TestSchemaEnsure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(*sql.DB) error

		wantInitialVersion int
	}{
		{
			name:    "ensure from fresh",
			prepare: func(_ *sql.DB) error { return nil },

			wantInitialVersion: 0,
		},
		{
			name: "update v1 to latest",
			prepare: func(db *sql.DB) error {
				return dbschema.SetupVersion(db, 1)
			},

			wantInitialVersion: 1,
		},
		{
			name: "update latest to latest",
			prepare: func(db *sql.DB) error {
				return dbschema.SetupVersion(db, dbschema.MaxVersion())
			},

			wantInitialVersion: dbschema.MaxVersion(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := databaseDir(t)
			db, err := sqlite.Open(tmpDir)
			require.NoError(t, err)

			err = tc.prepare(db.DB)
			require.NoError(t, err)

			require.Equal(t, tc.wantInitialVersion, schemaVersion(t, db.DB))

			ok, version, err := dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, dbschema.MaxVersion(), version)
			require.Equal(t, dbschema.MaxVersion(), schemaVersion(t, db.DB))
		})
	}
}

// schemaVersion returns the schema version of db, 0 without a schema.
func schemaVersion(t *testing.T, db *sql.DB) int {
	t.Helper()

	var version sql.NullInt64
	err := db.QueryRowContext(t.Context(), `SELECT MAX(version) FROM schema`).Scan(&version)
	if err != nil {
		return 0
	}

	return int(version.Int64)
}

func TestSchemaEnsure_normalizeServerIdentifiers(t *testing.T) {
	t.Parallel()

	tmpDir := databaseDir(t)
	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	err = dbschema.SetupVersion(db.DB, 41)
	require.NoError(t, err)

	_, err = db.Exec(`
INSERT INTO servers (id, name, type, connection_url, status, hardware_data, os_data, last_updated, channel_id, system_uuid, machine_id) VALUES
  (1, 'one',   'incus', 'https://one/',   'ready', '{}', '{}', '2025-03-12 10:57:43+00:00', 1, 'E9DE436E-B94E-4AEF-8563-883AEC84096E', 'E9DE436EB94E4AEF8563883AEC84096E'),
  (2, 'two',   'incus', 'https://two/',   'ready', '{}', '{}', '2025-03-12 10:57:43+00:00', 1, 'e9de436e-b94e-4aef-8563-883aec84096e', 'e9de436eb94e4aef8563883aec84096e'),
  (3, 'three', 'incus', 'https://three/', 'ready', '{}', '{}', '2025-03-12 10:57:43+00:00', 1, NULL, NULL);
`)
	require.NoError(t, err)

	_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)
	require.NoError(t, err)

	rows, err := db.Query(`SELECT id, system_uuid, machine_id FROM servers ORDER BY id`)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	type identifiers struct {
		systemUUID *string
		machineID  *string
	}

	got := map[int]identifiers{}
	for rows.Next() {
		var id int
		var ids identifiers
		require.NoError(t, rows.Scan(&id, &ids.systemUUID, &ids.machineID))
		got[id] = ids
	}

	require.NoError(t, rows.Err())

	lowerSystemUUID := "e9de436e-b94e-4aef-8563-883aec84096e"
	lowerMachineID := "e9de436eb94e4aef8563883aec84096e"

	require.Equal(t, identifiers{systemUUID: &lowerSystemUUID, machineID: &lowerMachineID}, got[1], "the oldest row of a case insensitive duplicate group keeps its identifiers, lower cased")
	require.Equal(t, identifiers{}, got[2], "the identifiers of the duplicate row are cleared, so the server can be matched again on its next registration")
	require.Equal(t, identifiers{}, got[3], "rows without identifiers are left untouched")
}

func TestSchemaEnsure_resourcesViewUUID(t *testing.T) {
	t.Parallel()

	tmpDir := databaseDir(t)
	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	err = dbschema.SetupVersion(db.DB, 42)
	require.NoError(t, err)

	_, err = db.Exec(`SELECT uuid FROM resources`)
	require.Error(t, err, "the resources view has no uuid column before v43")

	_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)
	require.NoError(t, err)

	_, err = db.Exec(`SELECT uuid FROM resources`)
	require.NoError(t, err, "the resources view has the uuid column from v43 onwards")
}

func TestSchemaEnsure_patchFile(t *testing.T) {
	t.Parallel()

	tmpDir := databaseDir(t)
	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)
	require.NoError(t, err)

	patchFile := filepath.Join(tmpDir, "patch.global.sql")

	// A failing patch is kept, so it is applied again on the next start.
	err = os.WriteFile(patchFile, []byte("INSERT INTO channels (name, description) VALUES ('patched', '');\nINSERT INTO missing VALUES (1);\n"), 0o600)
	require.NoError(t, err)

	_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)
	require.Error(t, err)
	require.FileExists(t, patchFile)

	// A committed patch is removed.
	err = os.WriteFile(patchFile, []byte("INSERT INTO channels (name, description) VALUES ('patched', '');\n"), 0o600)
	require.NoError(t, err)

	_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)
	require.NoError(t, err)
	require.NoFileExists(t, patchFile)

	var count int
	err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM channels WHERE name = 'patched'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestEnsureGlobal_migrateFromStandalone(t *testing.T) {
	t.Parallel()

	// More rows than fit in one transaction of the import, and more than cowsql handles in a single Exec.
	const rows = 2*dbschema.CopyRowsPerTx + 1

	backends := []struct {
		name string
		open func(t *testing.T, dir string) *sql.DB
	}{
		{
			name: "sqlite",
			open: func(t *testing.T, dir string) *sql.DB {
				t.Helper()

				db, err := sqlite.Open(dir)
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })

				return db.DB
			},
		},
		{
			name: "cowsql",
			open: cowsqltest.OpenEmpty,
		},
	}

	tests := []struct {
		name    string
		prepare func(t *testing.T, db *sql.DB)
	}{
		{
			name:    "empty global database",
			prepare: func(t *testing.T, db *sql.DB) { t.Helper() },
		},
		{
			name: "interrupted import",
			prepare: func(t *testing.T, db *sql.DB) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `
CREATE TABLE updates (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, uuid TEXT NOT NULL, UNIQUE (uuid));
INSERT INTO updates (uuid) VALUES ('uuid-1'), ('uuid-2');
`)
				require.NoError(t, err)
			},
		},
	}

	for _, backend := range backends {
		for _, tc := range tests {
			t.Run(backend.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				// Setup
				varDir := t.TempDir()

				standalone, err := sqlite.Open(varDir)
				require.NoError(t, err)

				// The schema version of Operations Center v0.10.1, before the clustering.
				err = dbschema.SetupVersion(standalone.DB, 43)
				require.NoError(t, err)

				// The deleted last row keeps the AUTOINCREMENT counter above the highest id.
				_, err = standalone.ExecContext(t.Context(), `
WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < ?)
INSERT INTO updates (uuid, origin, version, published_at, severity, upstream_channels, files)
  SELECT 'uuid-' || i, 'origin', i, '2025-03-12 10:57:43+00:00', 'none', '', '' FROM seq;
DELETE FROM updates WHERE id = ?;
`, rows, rows)
				require.NoError(t, err)

				var wantSeq int
				err = standalone.QueryRowContext(t.Context(), `SELECT seq FROM sqlite_sequence WHERE name = 'updates'`).Scan(&wantSeq)
				require.NoError(t, err)
				require.NoError(t, standalone.Close())

				// The global database of each start is new, in a database directory of the var dir.
				dir := filepath.Join(varDir, "database")
				require.NoError(t, os.Mkdir(dir, 0o700))

				db := backend.open(t, dir)
				tc.prepare(t, db)

				// Run test
				_, _, err = dbschema.EnsureGlobal(t.Context(), db, "", dir)
				require.NoError(t, err)

				// Assert
				var member string
				err = db.QueryRowContext(t.Context(), `SELECT name || '@' || address FROM cluster_members WHERE id = 1`).Scan(&member)
				require.NoError(t, err)
				require.Equal(t, "none@0.0.0.0", member, "expect the standalone member to exist from the start")

				var violations int
				err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&violations)
				require.NoError(t, err)
				require.Zero(t, violations, "expect the migrated rows to reference the standalone member")

				var count, uuids int
				err = db.QueryRowContext(t.Context(), `SELECT COUNT(*), COUNT(DISTINCT uuid) FROM updates`).Scan(&count, &uuids)
				require.NoError(t, err)
				require.Equal(t, rows-1, count, "expect all rows to be imported")
				require.Equal(t, rows-1, uuids, "expect no duplicated rows")

				var seq int
				err = db.QueryRowContext(t.Context(), `SELECT seq FROM sqlite_sequence WHERE name = 'updates'`).Scan(&seq)
				require.NoError(t, err)
				require.Equal(t, wantSeq, seq, "expect the AUTOINCREMENT counter of the standalone database")

				require.NoFileExists(t, filepath.Join(varDir, "local.db"), "expect the standalone database to be renamed")
				require.FileExists(t, filepath.Join(varDir, "local.db.migrated"), "expect the standalone database to be kept")

				// The global database of a member removed from the cluster starts empty.
				removedDir := filepath.Join(varDir, "database-removed")
				require.NoError(t, os.Mkdir(removedDir, 0o700))

				removedDB := backend.open(t, removedDir)

				_, _, err = dbschema.EnsureGlobal(t.Context(), removedDB, "10.0.0.1:8443", removedDir)
				require.NoError(t, err)

				err = removedDB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM updates`).Scan(&count)
				require.NoError(t, err)
				require.Zero(t, count, "expect the renamed standalone database not to be imported again")
			})
		}
	}
}

func TestEnsureGlobal_removedFirstMember(t *testing.T) {
	t.Parallel()

	dir := databaseDir(t)
	db, err := sqlite.Open(dir)
	require.NoError(t, err)

	_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", dir)
	require.NoError(t, err)

	// The first member bootstraps the cluster, a second one joins and the first one is removed.
	_, err = db.ExecContext(t.Context(), `
UPDATE cluster_members SET name = 'm1', address = '10.0.0.1:8443' WHERE id = 1;
INSERT INTO cluster_members (name, address, schema, description)
  SELECT 'm2', '10.0.0.2:8443', schema, '' FROM cluster_members WHERE id = 1;
DELETE FROM cluster_members WHERE id = 1;
`)
	require.NoError(t, err)

	_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "10.0.0.2:8443", dir)
	require.NoError(t, err)

	var names string
	err = db.QueryRowContext(t.Context(), `SELECT group_concat(name) FROM cluster_members`).Scan(&names)
	require.NoError(t, err)
	require.Equal(t, "m2", names)
}

// databaseDir returns a new database directory inside a var dir without a database of an older release.
func databaseDir(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "database")
	require.NoError(t, os.Mkdir(dir, 0o700))

	return dir
}
