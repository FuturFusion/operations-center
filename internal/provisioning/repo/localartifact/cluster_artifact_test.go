package localartifact_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsnet/golib/memfile"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	clusterEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestLocalArtifact_updateSignalHandler(t *testing.T) {
	const clusterName = "old"
	const existingClusterName = "existing"

	tests := []struct {
		name           string
		operation      lifecycle.ClusterUpdateOperation
		clusterName    string
		oldClusterName string

		assertLog log.MatcherFunc
	}{
		{
			name:           "success - rename",
			operation:      lifecycle.ClusterUpdateOperationRename,
			clusterName:    "new",
			oldClusterName: clusterName,

			assertLog: log.Noop,
		},
		{
			name:        "success - delete",
			operation:   lifecycle.ClusterUpdateOperationDelete,
			clusterName: clusterName,

			assertLog: log.Noop,
		},
		{
			name:      "skip - create operation",
			operation: lifecycle.ClusterUpdateOperationCreate,

			assertLog: log.Noop,
		},
		{
			name:           "skip - rename - old does not exist",
			operation:      lifecycle.ClusterUpdateOperationRename,
			clusterName:    "new",
			oldClusterName: "does_not_exist", // does not exist

			assertLog: log.Noop,
		},
		{
			name:           "error - rename - new does already exist",
			operation:      lifecycle.ClusterUpdateOperationRename,
			clusterName:    existingClusterName,
			oldClusterName: clusterName,

			assertLog: log.Contains("Failed to rename cluster artifact storage directory"),
		},
		{
			name:           "error - rename - new name outside of storage",
			operation:      lifecycle.ClusterUpdateOperationRename,
			clusterName:    "../escape",
			oldClusterName: clusterName,

			assertLog: log.Contains("Failed to rename cluster artifact storage directory"),
		},
		{
			name:           "error - rename - old name outside of storage",
			operation:      lifecycle.ClusterUpdateOperationRename,
			clusterName:    "new",
			oldClusterName: "..",

			assertLog: log.Contains("Failed to rename cluster artifact storage directory"),
		},
		{
			name:        "error - delete - parent directory",
			operation:   lifecycle.ClusterUpdateOperationDelete,
			clusterName: "..",

			assertLog: log.Contains("Failed to remove cluster artifact storage directory"),
		},
		{
			name:        "skip - delete - does not exist",
			operation:   lifecycle.ClusterUpdateOperationDelete,
			clusterName: "does_not_exist", // does not exist

			assertLog: log.Noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			parentDir := t.TempDir()
			tmpDir := filepath.Join(parentDir, "artifacts")
			err := os.Mkdir(tmpDir, 0o700)
			require.NoError(t, err)
			err = os.Mkdir(filepath.Join(tmpDir, clusterName), 0o700)
			require.NoError(t, err)
			err = os.Mkdir(filepath.Join(tmpDir, existingClusterName), 0o700)
			require.NoError(t, err)

			logBuf := &log.SyncBuffer{}
			err = logger.InitLogger(logBuf, "", false, false, false)
			require.NoError(t, err)

			oldClusterUpdateSignal := lifecycle.ClusterUpdateSignal
			lifecycle.ClusterUpdateSignal = signals.NewSync[lifecycle.ClusterUpdateMessage]()
			defer func() {
				lifecycle.ClusterUpdateSignal = oldClusterUpdateSignal
			}()

			localartifact.New(nil, fsroot.Open(t, tmpDir))
			t.Cleanup(lifecycle.ClusterUpdateSignal.Reset)

			// Run test
			lifecycle.ClusterUpdateSignal.Emit(t.Context(), lifecycle.ClusterUpdateMessage{
				Operation: tc.operation,
				Name:      tc.clusterName,
				OldName:   tc.oldClusterName,
			})

			// Assert
			tc.assertLog(t, logBuf)
			t.Log(logBuf.String())

			parentEntries, err := os.ReadDir(parentDir)
			require.NoError(t, err)
			require.Len(t, parentEntries, 1, "nothing outside of the storage directory must be touched")
			require.Equal(t, "artifacts", parentEntries[0].Name())
		})
	}
}

func TestLocalArtifactDatabaseActions(t *testing.T) {
	t.Parallel()

	clusterA := provisioning.Cluster{
		Name:    "clusterOne",
		Channel: "stable",
	}

	artifactOne := provisioning.ClusterArtifact{
		Cluster:     "clusterOne",
		Name:        "one",
		Description: "Some description",
		Properties: api.ConfigMap{
			"key": "value",
		},
	}

	artifactTwo := provisioning.ClusterArtifact{
		Cluster:     "clusterOne",
		Name:        "two",
		Description: "Some description about a file",
		Properties: api.ConfigMap{
			"key": "value",
		},
	}

	ctx := context.Background()

	// Create a new temporary directory.
	tmpDir := t.TempDir()

	// Seed source directory for artifact creation from path
	sourcePath := filepath.Join(tmpDir, "source")
	err := os.Mkdir(sourcePath, 0o700)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(sourcePath, "one.txt"), []byte(`one`), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(sourcePath, "two.txt"), []byte(`two with more content`), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(sourcePath, ".terraform.lock.hcl"), []byte(`ignored`), 0o600)
	require.NoError(t, err)
	// Create directory, should be skipped
	err = os.Mkdir(filepath.Join(sourcePath, "dir"), 0o700)
	require.NoError(t, err)

	// Setup DB
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	clusterStmts, err := clusterEntities.PrepareStmts(tx, false)
	require.NoError(t, err)

	artifactStmts, err := entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	ctx = clusterEntities.WithPreparedStmts(ctx, clusterStmts)
	ctx = entities.WithPreparedStmts(ctx, artifactStmts)

	cluster := sqlite.NewCluster(tx)

	// Add cluster
	_, err = cluster.Create(ctx, clusterA)
	require.NoError(t, err)

	// Add cluster artifact from path
	err = os.Mkdir(filepath.Join(tmpDir, "artifacts"), 0o700)
	require.NoError(t, err)
	artifactRepo := localartifact.New(tx, fsroot.Open(t, filepath.Join(tmpDir, "artifacts")))

	// Create artifact from directory
	_, err = artifactRepo.CreateClusterArtifactFromPath(ctx, artifactOne, sourcePath, []string{".terraform.lock.hcl"})
	require.NoError(t, err)

	// Create artifact from file
	_, err = artifactRepo.CreateClusterArtifactFromPath(ctx, artifactTwo, filepath.Join(sourcePath, "one.txt"), []string{})
	require.NoError(t, err)

	artifacts, err := artifactRepo.GetClusterArtifactAll(ctx, "clusterOne")
	require.NoError(t, err)
	require.Len(t, artifacts, 2)

	assertArtifactOne(t, &artifacts[0])

	names, err := artifactRepo.GetClusterArtifactAllNames(ctx, "clusterOne")
	require.NoError(t, err)
	require.Len(t, names, 2)

	artifact, err := artifactRepo.GetClusterArtifactByName(ctx, "clusterOne", "one")
	require.NoError(t, err)

	assertArtifactOne(t, artifact)

	zipArchiveType, ok := provisioning.ClusterArtifactArchiveTypes[provisioning.ClusterArtifactArchiveTypeExtZip]
	require.True(t, ok)

	rc, size, err := artifactRepo.GetClusterArtifactArchiveByName(ctx, "clusterOne", "one", zipArchiveType)
	require.NoError(t, err)
	require.Positive(t, size, 0) // we don't know the exact size of the zip archive, but it is required to be none 0.

	buf := bytes.Buffer{}
	n, err := file.SafeCopy(&buf, rc)
	require.NoError(t, err)
	require.Equal(t, int64(size), n)

	zipFile := memfile.New(buf.Bytes())

	zr, err := zip.NewReader(zipFile, int64(size))
	require.NoError(t, err)

	expectedFilesFound := map[string]bool{
		"one.txt": false,
		"two.txt": false,
	}

	for _, f := range zr.File {
		found, ok := expectedFilesFound[f.Name]
		require.True(t, ok, "unexpected file %q found in zip archive", f.Name)
		require.False(t, found, "file %q has already been seen", f.Name)

		expectedFilesFound[f.Name] = true
	}

	for filename, found := range expectedFilesFound {
		require.True(t, found, "file %q not found in zip archive", filename)
	}
}

func assertArtifactOne(t *testing.T, artifact *provisioning.ClusterArtifact) {
	t.Helper()

	require.Equal(t, "clusterOne", artifact.Cluster)
	require.Equal(t, "one", artifact.Name)
	require.Equal(t, "Some description", artifact.Description)
	require.Equal(t, api.ConfigMap{
		"key": "value",
	}, artifact.Properties)

	require.Len(t, artifact.Files, 2)
	require.Equal(t, "one.txt", artifact.Files[0].Name)
	require.Equal(t, "text/plain; charset=utf-8", artifact.Files[0].MimeType)
	require.Equal(t, int64(3), artifact.Files[0].Size)

	require.Equal(t, "two.txt", artifact.Files[1].Name)
	require.Equal(t, "text/plain; charset=utf-8", artifact.Files[1].MimeType)
	require.Equal(t, int64(21), artifact.Files[1].Size)

	r, err := artifact.Files[0].Open()
	require.NoError(t, err)

	data, err := io.ReadAll(r)
	require.NoError(t, err)

	require.Equal(t, []byte(`one`), data)
}

func TestLocalArtifact_invalidPath(t *testing.T) {
	zipArchiveType := provisioning.ClusterArtifactArchiveTypes[provisioning.ClusterArtifactArchiveTypeExtZip]

	tests := []struct {
		name string
		run  func(t *testing.T, repo provisioning.ClusterArtifactRepo, sourcePath string) error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "CreateClusterArtifactFromPath - cluster outside of storage",
			run: func(t *testing.T, repo provisioning.ClusterArtifactRepo, sourcePath string) error {
				t.Helper()
				_, err := repo.CreateClusterArtifactFromPath(t.Context(), provisioning.ClusterArtifact{Cluster: "../..", Name: "one"}, sourcePath, nil)
				return err
			},

			assertErr: errassert.ValidationErrorContains(`Invalid cluster artifact, "../.." is not a valid file name`),
		},
		{
			name: "CreateClusterArtifactFromPath - artifact name with path separator",
			run: func(t *testing.T, repo provisioning.ClusterArtifactRepo, sourcePath string) error {
				t.Helper()
				_, err := repo.CreateClusterArtifactFromPath(t.Context(), provisioning.ClusterArtifact{Cluster: "cluster", Name: "../../one"}, sourcePath, nil)
				return err
			},

			assertErr: errassert.ValidationErrorContains(`Invalid cluster artifact, "../../one" is not a valid file name`),
		},
		{
			name: "CreateClusterArtifactFromPath - source path does not exist",
			run: func(t *testing.T, repo provisioning.ClusterArtifactRepo, sourcePath string) error {
				t.Helper()
				_, err := repo.CreateClusterArtifactFromPath(t.Context(), provisioning.ClusterArtifact{Cluster: "cluster", Name: "one"}, filepath.Join(sourcePath, "missing"), nil)
				return err
			},

			assertErr: errassert.Is(os.ErrNotExist),
		},
		{
			name: "GetClusterArtifactArchiveByName - cluster is parent directory",
			run: func(t *testing.T, repo provisioning.ClusterArtifactRepo, sourcePath string) error {
				t.Helper()
				_, _, err := repo.GetClusterArtifactArchiveByName(t.Context(), "..", "source", zipArchiveType)
				return err
			},

			assertErr: errassert.ValidationErrorContains(`Invalid cluster artifact, ".." is not a valid file name`),
		},
		{
			name: "GetClusterArtifactArchiveByName - artifact name with path separator",
			run: func(t *testing.T, repo provisioning.ClusterArtifactRepo, sourcePath string) error {
				t.Helper()
				_, _, err := repo.GetClusterArtifactArchiveByName(t.Context(), "cluster", "../../source", zipArchiveType)
				return err
			},

			assertErr: errassert.ValidationErrorContains(`Invalid cluster artifact, "../../source" is not a valid file name`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			parentDir := t.TempDir()
			storageDir := filepath.Join(parentDir, "artifacts")
			err := os.Mkdir(storageDir, 0o700)
			require.NoError(t, err)

			sourcePath := filepath.Join(parentDir, "source")
			err = os.Mkdir(sourcePath, 0o700)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(sourcePath, "one.txt"), []byte(`one`), 0o600)
			require.NoError(t, err)

			oldClusterUpdateSignal := lifecycle.ClusterUpdateSignal
			lifecycle.ClusterUpdateSignal = signals.NewSync[lifecycle.ClusterUpdateMessage]()
			t.Cleanup(func() {
				lifecycle.ClusterUpdateSignal = oldClusterUpdateSignal
			})

			repo := localartifact.New(nil, fsroot.Open(t, storageDir))

			// Run test
			err = tc.run(t, repo, sourcePath)

			// Assert
			tc.assertErr(t, err)

			storageEntries, err := os.ReadDir(storageDir)
			require.NoError(t, err)
			require.Empty(t, storageEntries, "nothing must be created in the storage directory")

			parentEntries, err := os.ReadDir(parentDir)
			require.NoError(t, err)
			require.Len(t, parentEntries, 2, "nothing must be created outside of the storage directory")
		})
	}
}
