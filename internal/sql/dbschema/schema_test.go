package dbschema_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

func TestSchemaEnsure_incusImagesPerSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup string

		wantSources map[int]testIncusImageSource
		wantImages  map[int]testIncusImage
	}{
		{
			name: "success - source names are sanitized",
			setup: `
INSERT INTO incus_image_sources (id, name, url, filter_expression, last_updated) VALUES
  (1,  'images',  'https://one.example.com/',   'true', '2025-03-12 10:57:43+00:00'),
  (2,  'my-src',  'https://two.example.com/',   'true', '2025-03-12 10:57:43+00:00'),
  (3,  'my_src',  'https://three.example.com/', 'true', '2025-03-12 10:57:43+00:00'),
  (4,  'local',   'https://four.example.com/',  'true', '2025-03-12 10:57:43+00:00'),
  (5,  'sources', 'https://five.example.com/',  'true', '2025-03-12 10:57:43+00:00'),
  (6,  'a.b',     'https://six.example.com/',   'true', '2025-03-12 10:57:43+00:00'),
  (7,  'MySrc',   'https://seven.example.com/', 'true', '2025-03-12 10:57:43+00:00'),
  (8,  'Local',   'https://eight.example.com/', 'true', '2025-03-12 10:57:43+00:00'),
  (9,  'Images',  'https://nine.example.com/',  'true', '2025-03-12 10:57:43+00:00'),
  (10, 'Create',  'https://ten.example.com/',   'true', '2025-03-12 10:57:43+00:00');
`,

			wantSources: map[int]testIncusImageSource{
				1:  {name: "images", url: "https://one.example.com/"},
				2:  {name: "source_2", url: "https://two.example.com/"},
				3:  {name: "my_src", url: "https://three.example.com/"},
				4:  {name: "source_4", url: "https://four.example.com/"},
				5:  {name: "sources", url: "https://five.example.com/"},
				6:  {name: "a_b", url: "https://six.example.com/"},
				7:  {name: "mysrc", url: "https://seven.example.com/"},
				8:  {name: "source_8", url: "https://eight.example.com/"},
				9:  {name: "source_9", url: "https://nine.example.com/"},
				10: {name: "source_10", url: "https://ten.example.com/"},
				11: {name: "local", url: ""},
			},
			wantImages: map[int]testIncusImage{},
		},
		{
			name: "success - empty URL is replaced",
			setup: `
INSERT INTO incus_image_sources (id, name, url, filter_expression, last_updated) VALUES
  (1, 'images', '', 'true', '2025-03-12 10:57:43+00:00');
`,

			wantSources: map[int]testIncusImageSource{
				1: {name: "images", url: "invalid://source_1"},
				2: {name: "local", url: ""},
			},
			wantImages: map[int]testIncusImage{},
		},
		{
			name: "success - images are merged per source",
			setup: `
INSERT INTO incus_image_sources (id, name, url, filter_expression, last_updated) VALUES
  (1, 'images', 'https://one.example.com/', 'true', '2025-03-12 10:57:43+00:00');

INSERT INTO incus_images (id, name, description, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  (1, 'Debian:trixie:amd64:default', '', 'Debian',    'trixie', 'amd64', 'default', '{"v1":{"items":{"a":{}}},"v2":{"items":{"a":{}}}}', '2025-03-12 10:57:43+00:00', NULL),
  (2, 'debian:trixie:amd64:default', '', 'debian',    'trixie', 'amd64', 'default', '{"v2":{"items":{"b":{}}},"v3":{"items":{"b":{}}}}', '2025-03-12 10:57:43+00:00', NULL),
  (3, 'DEBIAN:trixie:amd64:default', '', 'DEBIAN',    'trixie', 'amd64', 'default', CAST('{"v4":{"items":{"c":{}}}}' AS BLOB),        '2025-03-12 10:57:43+00:00', 1),
  (4, 'AlmaLinux:10:amd64:Cloud',    '', 'AlmaLinux', '10',     'amd64', 'Cloud',   '{"v1":{"items":{"d":{}}}}',                     '2025-03-12 10:57:43+00:00', 1),
  (5, 'Alpine:3.22:amd64:default',   '', 'Alpine',    '3.22',   'amd64', 'default', '{"v1":{"items":{"e":{}}}}',                     '2025-03-12 10:57:43+00:00', NULL),
  (6, 'ALPINE:3.22:amd64:default',   '', 'ALPINE',    '3.22',   'amd64', 'default', '{"v1":{"items":{"f":{}}},"v2":{"items":{"f":{}}}}', '2025-03-12 10:57:43+00:00', NULL);
`,

			wantSources: map[int]testIncusImageSource{
				1: {name: "images", url: "https://one.example.com/"},
				2: {name: "local", url: ""},
			},
			wantImages: map[int]testIncusImage{
				// The row with the lower case name is kept. Its versions win.
				2: {source: "local", name: "debian:trixie:amd64:default", description: "", aliases: `[]`, versions: `{"v1":{"items":{"a":{}}},"v2":{"items":{"b":{}}},"v3":{"items":{"b":{}}}}`},
				// The same name can exist in different sources.
				3: {source: "images", name: "debian:trixie:amd64:default", description: "", aliases: ``, versions: `{"v4":{"items":{"c":{}}}}`},
				4: {source: "images", name: "almalinux:10:amd64:cloud", description: "", aliases: ``, versions: `{"v1":{"items":{"d":{}}}}`},
				// Without a row with the lower case name, the oldest row is kept.
				5: {source: "local", name: "alpine:3.22:amd64:default", description: "", aliases: `[]`, versions: `{"v1":{"items":{"e":{}}},"v2":{"items":{"f":{}}}}`},
			},
		},
		{
			name: "success - versions stored as blob are merged",
			setup: `
INSERT INTO incus_images (id, name, description, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  (1, 'debian:13:amd64:default', '', 'debian', '13', 'amd64', 'default', '{"v1":{"items":{"a":{}}}}',                     '2025-03-12 10:57:43+00:00', NULL),
  (2, 'Debian:13:amd64:default', '', 'Debian', '13', 'amd64', 'default', CAST('{"v2":{"items":{"b":{}}}}' AS BLOB), '2025-03-12 10:57:43+00:00', NULL);
`,

			wantSources: map[int]testIncusImageSource{
				1: {name: "local", url: ""},
			},
			wantImages: map[int]testIncusImage{
				1: {source: "local", name: "debian:13:amd64:default", description: "", aliases: `[]`, versions: `{"v1":{"items":{"a":{}}},"v2":{"items":{"b":{}}}}`},
			},
		},
		{
			name: "success - invalid versions are dropped by the merge",
			setup: `
INSERT INTO incus_images (id, name, description, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  (1, 'debian:13:amd64:default', '', 'debian', '13',   'amd64', 'default', 'invalid',                   '2025-03-12 10:57:43+00:00', NULL),
  (2, 'Debian:13:amd64:default', '', 'Debian', '13',   'amd64', 'default', '{"v1":{"items":{"a":{}}}}', '2025-03-12 10:57:43+00:00', NULL),
  (3, 'Alpine:edge:amd64:cloud', '', 'Alpine', 'edge', 'amd64', 'cloud',   '{"v2":{"items":{"b":{}}}}', '2025-03-12 10:57:43+00:00', NULL),
  (4, 'ALPINE:edge:amd64:cloud', '', 'ALPINE', 'edge', 'amd64', 'cloud',   'invalid',                   '2025-03-12 10:57:43+00:00', NULL);
`,

			wantSources: map[int]testIncusImageSource{
				1: {name: "local", url: ""},
			},
			wantImages: map[int]testIncusImage{
				// The invalid versions of the kept row are replaced.
				1: {source: "local", name: "debian:13:amd64:default", description: "", aliases: `[]`, versions: `{"v1":{"items":{"a":{}}}}`},
				// The invalid versions of the dropped row are lost.
				3: {source: "local", name: "alpine:edge:amd64:cloud", description: "", aliases: `[]`, versions: `{"v2":{"items":{"b":{}}}}`},
			},
		},
		{
			name: "success - aliases and descriptions are merged",
			setup: `
INSERT INTO incus_images (id, name, description, aliases, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  (1, 'Ubuntu:noble:amd64:cloud', '',             '["ubuntu/noble","ubuntu/lts"]', 'Ubuntu', 'noble', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00', NULL),
  (2, 'ubuntu:noble:amd64:cloud', '',             '["ubuntu/24.04","ubuntu/lts"]', 'ubuntu', 'noble', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00', NULL),
  (3, 'UBUNTU:noble:amd64:cloud', 'Ubuntu Noble', '',                              'UBUNTU', 'noble', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00', NULL);
`,

			wantSources: map[int]testIncusImageSource{
				1: {name: "local", url: ""},
			},
			wantImages: map[int]testIncusImage{
				// The aliases of the preferred row come first. The first description that is not empty wins.
				2: {source: "local", name: "ubuntu:noble:amd64:cloud", description: "Ubuntu Noble", aliases: `["ubuntu/24.04","ubuntu/lts","ubuntu/noble"]`, versions: `{}`},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := databaseDir(t)
			db, err := sqlite.Open(tmpDir)
			require.NoError(t, err, "open database")

			err = dbschema.SetupVersion(db.DB, 44)
			require.NoError(t, err, "setup schema version 44")

			_, err = db.Exec(tc.setup)
			require.NoError(t, err, "insert test data")

			// Run test
			_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)

			// Assert
			require.NoError(t, err, "update to the latest schema version")
			require.Equal(t, tc.wantSources, getTestIncusImageSources(t, db.DB), "image sources after the update")
			require.Equal(t, tc.wantImages, getTestIncusImages(t, db.DB), "incus images after the update")
		})
	}
}

func TestSchemaEnsure_incusImagesPerSourceConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		insert string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - same name in another source",
			insert: `
INSERT INTO incus_images (name, description, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  ('almalinux:10:amd64:cloud', '', 'almalinux', '10', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00', (SELECT id FROM incus_image_sources WHERE name = 'local'));
`,

			assertErr: require.NoError,
		},
		{
			name: "error - same name in the same source",
			insert: `
INSERT INTO incus_images (name, description, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  ('almalinux:10:amd64:cloud', '', 'almalinux', '10', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00', 1);
`,

			assertErr: require.Error,
		},
		{
			name: "error - image without source",
			insert: `
INSERT INTO incus_images (name, description, operating_system, release, architecture, variant, versions, last_updated) VALUES
  ('rocky:9:amd64:cloud', '', 'rocky', '9', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00');
`,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := databaseDir(t)
			db, err := sqlite.Open(tmpDir)
			require.NoError(t, err, "open database")

			err = dbschema.SetupVersion(db.DB, 44)
			require.NoError(t, err, "setup schema version 44")

			_, err = db.Exec(`
INSERT INTO incus_image_sources (id, name, url, filter_expression, last_updated) VALUES
  (1, 'images', 'https://one.example.com/', 'true', '2025-03-12 10:57:43+00:00');

INSERT INTO incus_images (id, name, description, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  (1, 'almalinux:10:amd64:cloud', '', 'almalinux', '10', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00', 1);
`)
			require.NoError(t, err, "insert test data")

			_, _, err = dbschema.EnsureGlobal(t.Context(), db.DB, "", tmpDir)
			require.NoError(t, err, "update to the latest schema version")

			// Run test
			_, err = db.Exec(tc.insert)

			// Assert
			tc.assertErr(t, err, "from v45 onwards, every image has a source and the name is unique per source")
		})
	}
}

type testIncusImageSource struct {
	name string
	url  string
}

func getTestIncusImageSources(t *testing.T, db *sql.DB) map[int]testIncusImageSource {
	t.Helper()

	rows, err := db.Query(`SELECT id, name, url FROM incus_image_sources`)
	require.NoError(t, err, "query image sources")

	defer func() { _ = rows.Close() }()

	sources := map[int]testIncusImageSource{}
	for rows.Next() {
		var id int
		var src testIncusImageSource
		err = rows.Scan(&id, &src.name, &src.url)
		require.NoError(t, err, "scan image source")

		sources[id] = src
	}

	require.NoError(t, rows.Err(), "read image sources")

	return sources
}

type testIncusImage struct {
	source      string
	name        string
	description string
	aliases     string
	versions    string
}

func getTestIncusImages(t *testing.T, db *sql.DB) map[int]testIncusImage {
	t.Helper()

	rows, err := db.Query(`
SELECT incus_images.id, incus_image_sources.name, incus_images.name, incus_images.description, CAST(incus_images.aliases AS TEXT), CAST(incus_images.versions AS TEXT)
  FROM incus_images
  JOIN incus_image_sources ON incus_images.incus_image_source_id = incus_image_sources.id`)
	require.NoError(t, err, "query incus images")

	defer func() { _ = rows.Close() }()

	images := map[int]testIncusImage{}
	for rows.Next() {
		var id int
		var img testIncusImage
		err = rows.Scan(&id, &img.source, &img.name, &img.description, &img.aliases, &img.versions)
		require.NoError(t, err, "scan incus image")

		// Sort the keys, so the versions can be compared as string.
		var versions any
		err = json.Unmarshal([]byte(img.versions), &versions)
		require.NoError(t, err, "versions of incus image %d are valid JSON", id)

		sorted, err := json.Marshal(versions)
		require.NoError(t, err, "marshal versions of incus image %d", id)

		img.versions = string(sorted)
		images[id] = img
	}

	require.NoError(t, rows.Err(), "read incus images")

	return images
}
