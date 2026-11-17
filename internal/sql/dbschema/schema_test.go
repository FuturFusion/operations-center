package dbschema_test

import (
	"context"
	"database/sql"
	"encoding/json"
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

			err = tc.prepare(db)
			require.NoError(t, err)

			schemaVersion, err := dbschema.Ensure(context.Background(), db, tmpDir)
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

	err = dbschema.SetupVersion(db, 41)
	require.NoError(t, err)

	_, err = db.Exec(`
INSERT INTO servers (id, name, type, connection_url, status, hardware_data, os_data, last_updated, channel_id, system_uuid, machine_id) VALUES
  (1, 'one',   'incus', 'https://one/',   'ready', '{}', '{}', '2025-03-12 10:57:43+00:00', 1, 'E9DE436E-B94E-4AEF-8563-883AEC84096E', 'E9DE436EB94E4AEF8563883AEC84096E'),
  (2, 'two',   'incus', 'https://two/',   'ready', '{}', '{}', '2025-03-12 10:57:43+00:00', 1, 'e9de436e-b94e-4aef-8563-883aec84096e', 'e9de436eb94e4aef8563883aec84096e'),
  (3, 'three', 'incus', 'https://three/', 'ready', '{}', '{}', '2025-03-12 10:57:43+00:00', 1, NULL, NULL);
`)
	require.NoError(t, err)

	_, err = dbschema.Ensure(context.Background(), db, tmpDir)
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

	err = dbschema.SetupVersion(db, 42)
	require.NoError(t, err)

	_, err = db.Exec(`SELECT uuid FROM resources`)
	require.Error(t, err, "the resources view has no uuid column before v43")

	_, err = dbschema.Ensure(context.Background(), db, tmpDir)
	require.NoError(t, err)

	_, err = db.Exec(`SELECT uuid FROM resources`)
	require.NoError(t, err, "the resources view has the uuid column from v43 onwards")
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
			tmpDir := t.TempDir()
			db, err := sqlite.Open(tmpDir)
			require.NoError(t, err, "open database")

			err = dbschema.SetupVersion(db, 43)
			require.NoError(t, err, "setup schema version 43")

			_, err = db.Exec(tc.setup)
			require.NoError(t, err, "insert test data")

			// Run test
			_, err = dbschema.Ensure(t.Context(), db, tmpDir)

			// Assert
			require.NoError(t, err, "update to the latest schema version")
			require.Equal(t, tc.wantSources, getTestIncusImageSources(t, db), "image sources after the update")
			require.Equal(t, tc.wantImages, getTestIncusImages(t, db), "incus images after the update")
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
			tmpDir := t.TempDir()
			db, err := sqlite.Open(tmpDir)
			require.NoError(t, err, "open database")

			err = dbschema.SetupVersion(db, 43)
			require.NoError(t, err, "setup schema version 43")

			_, err = db.Exec(`
INSERT INTO incus_image_sources (id, name, url, filter_expression, last_updated) VALUES
  (1, 'images', 'https://one.example.com/', 'true', '2025-03-12 10:57:43+00:00');

INSERT INTO incus_images (id, name, description, operating_system, release, architecture, variant, versions, last_updated, incus_image_source_id) VALUES
  (1, 'almalinux:10:amd64:cloud', '', 'almalinux', '10', 'amd64', 'cloud', '{}', '2025-03-12 10:57:43+00:00', 1);
`)
			require.NoError(t, err, "insert test data")

			_, err = dbschema.Ensure(t.Context(), db, tmpDir)
			require.NoError(t, err, "update to the latest schema version")

			// Run test
			_, err = db.Exec(tc.insert)

			// Assert
			tc.assertErr(t, err, "from v44 onwards, every image has a source and the name is unique per source")
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
