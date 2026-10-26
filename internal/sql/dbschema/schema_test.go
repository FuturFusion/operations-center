package dbschema_test

import (
	"context"
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

		wantSchemaVersion int
	}{
		{
			name:    "ensure from fresh",
			prepare: func(_ *sql.DB) error { return nil },

			wantSchemaVersion: 0,
		},
		{
			name: "update v1 to latest",
			prepare: func(db *sql.DB) error {
				return dbschema.SetupVersion(db, 1)
			},

			wantSchemaVersion: 1,
		},
		{
			name: "update latest to latest",
			prepare: func(db *sql.DB) error {
				return dbschema.SetupVersion(db, dbschema.MaxVersion())
			},

			wantSchemaVersion: dbschema.MaxVersion(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			db, err := sqlite.Open(tmpDir)
			require.NoError(t, err)

			err = tc.prepare(db.DB)
			require.NoError(t, err)

			schemaVersion, err := dbschema.EnsureLocal(context.Background(), db.DB, tmpDir, dbschema.Updates(), dbschema.FreshSchema())
			require.NoError(t, err)
			require.Equal(t, tc.wantSchemaVersion, schemaVersion)
		})
	}
}

func TestSchemaEnsure_normalizeServerIdentifiers(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
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

	_, err = dbschema.EnsureLocal(context.Background(), db.DB, tmpDir, dbschema.Updates(), dbschema.FreshSchema())
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

	tmpDir := t.TempDir()
	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	err = dbschema.SetupVersion(db.DB, 42)
	require.NoError(t, err)

	_, err = db.Exec(`SELECT uuid FROM resources`)
	require.Error(t, err, "the resources view has no uuid column before v43")

	_, err = dbschema.EnsureLocal(context.Background(), db.DB, tmpDir, dbschema.Updates(), dbschema.FreshSchema())
	require.NoError(t, err)

	_, err = db.Exec(`SELECT uuid FROM resources`)
	require.NoError(t, err, "the resources view has the uuid column from v43 onwards")
}

func TestSchemaEnsure_patchFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	db, err := sqlite.Open(tmpDir)
	require.NoError(t, err)

	_, err = dbschema.EnsureLocal(t.Context(), db.DB, tmpDir, dbschema.Updates(), dbschema.FreshSchema())
	require.NoError(t, err)

	patchFile := filepath.Join(tmpDir, "patch.local.sql")

	// A failing patch is kept, so it is applied again on the next start.
	err = os.WriteFile(patchFile, []byte("INSERT INTO channels (name, description) VALUES ('patched', '');\nINSERT INTO missing VALUES (1);\n"), 0o600)
	require.NoError(t, err)

	_, err = dbschema.EnsureLocal(t.Context(), db.DB, tmpDir, dbschema.Updates(), dbschema.FreshSchema())
	require.Error(t, err)
	require.FileExists(t, patchFile)

	// A committed patch is removed.
	err = os.WriteFile(patchFile, []byte("INSERT INTO channels (name, description) VALUES ('patched', '');\n"), 0o600)
	require.NoError(t, err)

	_, err = dbschema.EnsureLocal(t.Context(), db.DB, tmpDir, dbschema.Updates(), dbschema.FreshSchema())
	require.NoError(t, err)
	require.NoFileExists(t, patchFile)

	var count int
	err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM channels WHERE name = 'patched'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestEnsureGlobal_migrateFromStandalone(t *testing.T) {
	varDir := t.TempDir()
	t.Setenv("OPERATIONS_CENTER_DIR", varDir)

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

	countUpdates := func(dir string, address string) int {
		db, err := sqlite.Open(dir)
		require.NoError(t, err)

		_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, address, dir)
		require.NoError(t, err)

		var count int
		err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM updates`).Scan(&count)
		require.NoError(t, err)

		return count
	}

	// The standalone database is imported once and kept renamed.
	require.Equal(t, 1, countUpdates(t.TempDir(), ""))
	require.NoFileExists(t, filepath.Join(varDir, "local.db"))
	require.FileExists(t, filepath.Join(varDir, "local.db.migrated"))

	// The global database of a member removed from the cluster starts empty.
	require.Equal(t, 0, countUpdates(t.TempDir(), "10.0.0.1:8443"))
}
