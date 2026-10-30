package dbschema_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

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

	varDir := t.TempDir()

	standalone, err := sqlite.Open(varDir)
	require.NoError(t, err)

	// The schema version of Operations Center v0.10.1, before the clustering.
	err = dbschema.SetupVersion(standalone.DB, 43)
	require.NoError(t, err)

	_, err = standalone.Exec(`
INSERT INTO updates (uuid, origin, version, published_at, severity, upstream_channels, files) VALUES
  ('b32d0079-c48b-4957-b1cb-bef54125c861', 'origin', '1', '2025-03-12 10:57:43+00:00', 'none', '', '');
`)
	require.NoError(t, err)
	require.NoError(t, standalone.Close())

	// The global database of each start is new, in the database directory of the var dir.
	countUpdates := func(address string) int {
		dir := filepath.Join(varDir, "database")
		require.NoError(t, os.RemoveAll(dir))
		require.NoError(t, os.Mkdir(dir, 0o700))

		db, err := sqlite.Open(dir)
		require.NoError(t, err)

		_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, address, dir)
		require.NoError(t, err)

		// The standalone member exists from the start, so the files stored by it reference it.
		var member string
		err = db.QueryRowContext(t.Context(), `SELECT name || '@' || address FROM cluster_members WHERE id = 1`).Scan(&member)
		require.NoError(t, err)
		require.Equal(t, "none@0.0.0.0", member)

		var violations int
		err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&violations)
		require.NoError(t, err)
		require.Zero(t, violations, "the migrated rows reference the standalone member")

		var count int
		err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM updates`).Scan(&count)
		require.NoError(t, err)

		return count
	}

	// The standalone database is imported once and kept renamed.
	require.Equal(t, 1, countUpdates(""))
	require.NoFileExists(t, filepath.Join(varDir, "local.db"))
	require.FileExists(t, filepath.Join(varDir, "local.db.migrated"))

	// The global database of a member removed from the cluster starts empty.
	require.Equal(t, 0, countUpdates("10.0.0.1:8443"))
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
INSERT INTO cluster_members (name, address, schema, api_extensions, description, architecture)
  SELECT 'm2', '10.0.0.2:8443', schema, api_extensions, '', architecture FROM cluster_members WHERE id = 1;
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
