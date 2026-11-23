package patches

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_patchRemoveVarLibOperationCenterTerraformDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)

		wantEntries []string
	}{
		{
			name: "success - directories exist",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				for _, name := range []string{"servercerts", "terraform/cluster-configs/cluster", "updates"} {
					err := os.MkdirAll(filepath.Join(dir, name), 0o700)
					require.NoError(t, err)
				}

				err := os.WriteFile(filepath.Join(dir, "servercerts", "cluster.crt"), nil, 0o600)
				require.NoError(t, err)
			},

			wantEntries: []string{"updates"},
		},
		{
			name: "success - directories do not exist",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.Mkdir(filepath.Join(dir, "updates"), 0o700)
				require.NoError(t, err)
			},

			wantEntries: []string{"updates"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setup(t, tmpDir)

			// Run test
			err := patchRemoveVarLibOperationCenterTerraformDirs(t.Context(), "test", nil, fsroot.Open(t, tmpDir))

			// Assert
			require.NoError(t, err)

			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)
			gotEntries := make([]string, 0, len(entries))
			for _, entry := range entries {
				gotEntries = append(gotEntries, entry.Name())
			}

			require.Equal(t, tc.wantEntries, gotEntries, "only the servercerts and terraform directories are removed from the var directory")
		})
	}
}

func Test_patchIncusImagesPerSourceDirectory(t *testing.T) {
	t.Parallel()

	oldImageDir := filepath.Join("alpine", "3.22", "amd64", "default")

	tests := []struct {
		name  string
		setup func(t *testing.T, ctx context.Context, db dbdriver.DBTX, dir string)

		wantFiles    []string
		wantVersions map[string][]string
	}{
		{
			name: "success - move from the old layout, files without version are removed, other entries stay",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX, dir string) {
				t.Helper()
				for _, version := range []string{"v1", "v2", "v3"} {
					writeTestFile(t, filepath.Join(dir, "images", oldImageDir, version, "rootfs"))
				}

				writeTestFile(t, filepath.Join(dir, "images", "debian", "13", "amd64", "default", "v8", "rootfs"))
				writeTestFile(t, filepath.Join(dir, "images", "lost+found", "file"))
				writeTestFile(t, filepath.Join(dir, "images", "notes.txt"))
			},

			wantFiles: []string{
				"images/alpine/alpine/3.22/amd64/default/v3/rootfs",
				"images/local/alpine/3.22/amd64/default/v1/rootfs",
				"images/local/alpine/3.22/amd64/default/v2/rootfs",
				"images/lost+found/file",
				"images/notes.txt",
			},
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"alpine/alpine:3.22:amd64:default": {"v3"},
				"local/debian:13:amd64:default":    nil,
			},
		},
		{
			name: "success - rerun after a partial move",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX, dir string) {
				t.Helper()
				writeTestFile(t, filepath.Join(dir, "images", "local", oldImageDir, "v1", "rootfs"))
				for _, version := range []string{"v2", "v3"} {
					writeTestFile(t, filepath.Join(dir, "images", ".v1", oldImageDir, version, "rootfs"))
				}
			},

			wantFiles: []string{
				"images/alpine/alpine/3.22/amd64/default/v3/rootfs",
				"images/local/alpine/3.22/amd64/default/v1/rootfs",
				"images/local/alpine/3.22/amd64/default/v2/rootfs",
			},
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"alpine/alpine:3.22:amd64:default": {"v3"},
				"local/debian:13:amd64:default":    nil,
			},
		},
		{
			name: "success - rerun after a partial move of the old tree",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX, dir string) {
				t.Helper()
				for _, version := range []string{"v1", "v2", "v3"} {
					writeTestFile(t, filepath.Join(dir, "images", ".v1.partial", oldImageDir, version, "rootfs"))
				}

				writeTestFile(t, filepath.Join(dir, "images", "debian", "13", "amd64", "default", "v9", "rootfs"))
			},

			wantFiles: []string{
				"images/alpine/alpine/3.22/amd64/default/v3/rootfs",
				"images/local/alpine/3.22/amd64/default/v1/rootfs",
				"images/local/alpine/3.22/amd64/default/v2/rootfs",
				"images/local/debian/13/amd64/default/v9/rootfs",
			},
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"alpine/alpine:3.22:amd64:default": {"v3"},
				"local/debian:13:amd64:default":    {"v9"},
			},
		},
		{
			name: "success - invalid paths stored by an earlier version",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX, dir string) {
				t.Helper()
				for _, version := range []string{"v1", "v2", "v3"} {
					writeTestFile(t, filepath.Join(dir, "images", oldImageDir, version, "rootfs"))
				}

				writeTestFile(t, filepath.Join(dir, "images", "alpine", "edge", "amd64", "default", "v1", "rootfs"))
				writeTestFile(t, filepath.Join(dir, "secret"))

				createTestIncusImage(t, ctx, db, image.IncusImage{
					Source: api.IncusImageSourceLocal, Name: "alpine:edge:amd64:default", OperatingSystem: "alpine", Release: "edge", Architecture: "amd64", Variant: "default",
					Versions: testIncusImageVersions("v1", "../../../../../secret"),
				})

				createTestIncusImage(t, ctx, db, image.IncusImage{
					Source: "alpine", Name: "..:..:amd64:default", OperatingSystem: "..", Release: "..", Architecture: "amd64", Variant: "default",
					Versions: testIncusImageVersions("v4"),
				})
			},

			wantFiles: []string{
				"images/alpine/alpine/3.22/amd64/default/v3/rootfs",
				"images/local/alpine/3.22/amd64/default/v1/rootfs",
				"images/local/alpine/3.22/amd64/default/v2/rootfs",
				"images/local/alpine/edge/amd64/default/v1/rootfs",
				"secret",
			},
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"local/alpine:edge:amd64:default":  {"v1"},
				"alpine/alpine:3.22:amd64:default": {"v3"},
				"alpine/..:..:amd64:default":       nil,
				"local/debian:13:amd64:default":    nil,
			},
		},
		{
			name: "success - no images directory",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX, dir string) {
				t.Helper()
			},

			wantFiles: []string{},
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"alpine/alpine:3.22:amd64:default": {"v2", "v3"},
				"local/debian:13:amd64:default":    {"v9"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			ctx, db := setupIncusImagesDB(t)
			tc.setup(t, ctx, db, tmpDir)

			// Run test
			err := transaction.Do(ctx, func(ctx context.Context) error {
				return patchIncusImagesPerSourceDirectory(ctx, "test", db, fsroot.Open(t, tmpDir))
			})

			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.wantFiles, listTestFiles(t, tmpDir), "the files are in the directory of their source and the old tree is removed")
			require.Equal(t, tc.wantVersions, getTestIncusImageVersions(t, ctx, db), "versions without files are removed from the image")
		})
	}
}

func Test_patchApplyOnce_incusImagesPerSourceDirectory(t *testing.T) {
	t.Parallel()

	oldImageDir := filepath.Join("alpine", "3.22", "amd64", "default")

	tests := []struct {
		name  string
		setup func(t *testing.T, ctx context.Context, db dbdriver.DBTX)
		run   patchRun

		assertErr        require.ErrorAssertionFunc
		wantApplied      []string
		wantOldDirExists bool
		wantVersions     map[string][]string
	}{
		{
			name: "success",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX) {
				t.Helper()
			},
			run: patchIncusImagesPerSourceDirectory,

			assertErr:        require.NoError,
			wantApplied:      []string{"test"},
			wantOldDirExists: false,
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"alpine/alpine:3.22:amd64:default": {"v3"},
				"local/debian:13:amd64:default":    nil,
			},
		},
		{
			name: "error - patch fails after moving the files",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX) {
				t.Helper()
			},
			run: func(ctx context.Context, name string, db dbdriver.DBTX, varRoot *os.Root) error {
				err := patchIncusImagesPerSourceDirectory(ctx, name, db, varRoot)
				if err != nil {
					return err
				}

				return boom.Error
			},

			assertErr:        boom.ErrorIs,
			wantApplied:      nil,
			wantOldDirExists: true,
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"alpine/alpine:3.22:amd64:default": {"v2", "v3"},
				"local/debian:13:amd64:default":    {"v9"},
			},
		},
		{
			name: "success - patch claimed by another member",
			setup: func(t *testing.T, ctx context.Context, db dbdriver.DBTX) {
				t.Helper()
				err := MarkApplied(ctx, db, "test")
				require.NoError(t, err)
			},
			run: patchIncusImagesPerSourceDirectory,

			assertErr:        require.NoError,
			wantApplied:      []string{"test"},
			wantOldDirExists: false,
			wantVersions: map[string][]string{
				"local/alpine:3.22:amd64:default":  {"v1", "v2"},
				"alpine/alpine:3.22:amd64:default": {"v2", "v3"},
				"local/debian:13:amd64:default":    {"v9"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			for _, version := range []string{"v1", "v2", "v3"} {
				writeTestFile(t, filepath.Join(tmpDir, "images", oldImageDir, version, "rootfs"))
			}

			ctx, db := setupIncusImagesDB(t)
			tc.setup(t, ctx, db)

			p := patch{name: "test", stage: PreSecurityInfrastructure, scope: scopeCluster, run: tc.run}

			// Run test
			err := p.applyOnce(ctx, db, fsroot.Open(t, tmpDir))

			// Assert
			tc.assertErr(t, err)

			gotApplied, err := getAppliedPatches(ctx, db)
			require.NoError(t, err)
			require.Equal(t, tc.wantApplied, gotApplied, "the patch is only recorded as applied, if it succeeds or another member claimed it")

			_, err = os.Stat(filepath.Join(tmpDir, "images", ".v1"))
			require.Equal(t, tc.wantOldDirExists, err == nil, "the old tree is only created by the patch and only removed after the commit")

			require.Equal(t, tc.wantVersions, getTestIncusImageVersions(t, ctx, db), "the removal of versions without files is rolled back on error and skipped for a claimed patch")
		})
	}
}

func listTestFiles(t *testing.T, dir string) []string {
	t.Helper()

	files := []string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	require.NoError(t, err)

	return files
}

func getTestIncusImageVersions(t *testing.T, ctx context.Context, db dbdriver.DBTX) map[string][]string {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
SELECT incus_image_sources.name, incus_images.name, incus_images.versions
  FROM incus_images
  JOIN incus_image_sources ON incus_images.incus_image_source_id = incus_image_sources.id`)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	versions := map[string][]string{}
	for rows.Next() {
		var source, name string
		var imgVersions api.IncusImageVersions
		err = rows.Scan(&source, &name, &imgVersions)
		require.NoError(t, err)

		versions[source+"/"+name] = slices.Sorted(maps.Keys(imgVersions))
	}

	require.NoError(t, rows.Err())

	return versions
}

// setupIncusImagesDB creates the tables used by the patch. The package can not
// use dbschema, because dbschema imports it.
func setupIncusImagesDB(t *testing.T) (context.Context, dbdriver.DBTX) {
	t.Helper()

	ctx := context.Background()

	db, err := dbdriver.Open(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = db.ExecContext(ctx, `
CREATE TABLE patches (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  name VARCHAR(255) NOT NULL,
  applied_at DATETIME NOT NULL,
  UNIQUE (name)
);

CREATE TABLE incus_image_sources (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE (name)
);

CREATE TABLE incus_images (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  name TEXT NOT NULL,
  operating_system TEXT NOT NULL,
  release TEXT NOT NULL,
  architecture TEXT NOT NULL,
  variant TEXT NOT NULL,
  versions TEXT NOT NULL,
  incus_image_source_id INTEGER NOT NULL,
  UNIQUE (incus_image_source_id, name),
  FOREIGN KEY (incus_image_source_id) REFERENCES incus_image_sources(id)
);

INSERT INTO incus_image_sources (name) VALUES ('local'), ('alpine');
`)
	require.NoError(t, err)

	dbWithTransaction := transaction.Enable(db)

	images := []image.IncusImage{
		{Source: "alpine", Name: "alpine:3.22:amd64:default", OperatingSystem: "alpine", Release: "3.22", Architecture: "amd64", Variant: "default", Versions: testIncusImageVersions("v2", "v3")},
		{Source: api.IncusImageSourceLocal, Name: "alpine:3.22:amd64:default", OperatingSystem: "Alpine", Release: "3.22", Architecture: "amd64", Variant: "default", Versions: testIncusImageVersions("v1", "v2")},
		{Source: api.IncusImageSourceLocal, Name: "debian:13:amd64:default", OperatingSystem: "debian", Release: "13", Architecture: "amd64", Variant: "default", Versions: testIncusImageVersions("v9")},
	}

	for _, img := range images {
		createTestIncusImage(t, ctx, dbWithTransaction, img)
	}

	return ctx, dbWithTransaction
}

func createTestIncusImage(t *testing.T, ctx context.Context, db dbdriver.DBTX, img image.IncusImage) {
	t.Helper()

	_, err := db.ExecContext(ctx, `
INSERT INTO incus_images (name, operating_system, release, architecture, variant, versions, incus_image_source_id)
  VALUES (?, ?, ?, ?, ?, ?, (SELECT id FROM incus_image_sources WHERE name = ?))`,
		img.Name, img.OperatingSystem, img.Release, img.Architecture, img.Variant, img.Versions, img.Source)
	require.NoError(t, err, "create incus image %s/%s", img.Source, img.Name)
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(path), 0o700)
	require.NoError(t, err)

	err = os.WriteFile(path, nil, 0o600)
	require.NoError(t, err)
}

func testIncusImageVersions(identifiers ...string) api.IncusImageVersions {
	versions := api.IncusImageVersions{}
	for _, identifier := range identifiers {
		versions[identifier] = api.IncusImageVersion{
			Items: map[string]api.IncusImageVersionItem{
				"rootfs": {},
			},
		}
	}

	return versions
}
