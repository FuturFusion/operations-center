package localartifact_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsnet/golib/memfile"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	clusterEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestLocalArtifact_updateSignalHandler(t *testing.T) {
	const clusterName = "old"
	const existingClusterName = "existing"

	noLogAssert := func(t *testing.T, logBuf *bytes.Buffer) {
		t.Helper()
	}

	logContains := func(want string) func(t *testing.T, logBuf *bytes.Buffer) {
		return func(t *testing.T, logBuf *bytes.Buffer) {
			t.Helper()

			// Give logs a little bit of time to be processed.
			for range 5 {
				if strings.Contains(logBuf.String(), want) {
					break
				}

				time.Sleep(10 * time.Millisecond)
			}

			require.Contains(t, logBuf.String(), want)
		}
	}

	tests := []struct {
		name           string
		operation      provisioning.ClusterUpdateOperation
		clusterName    string
		oldClusterName string

		assertLog func(t *testing.T, logBuf *bytes.Buffer)
	}{
		{
			name:           "success - rename",
			operation:      provisioning.ClusterUpdateOperationRename,
			clusterName:    "new",
			oldClusterName: clusterName,

			assertLog: noLogAssert,
		},
		{
			name:        "success - delete",
			operation:   provisioning.ClusterUpdateOperationDelete,
			clusterName: clusterName,

			assertLog: noLogAssert,
		},
		{
			name:      "skip - create operation",
			operation: provisioning.ClusterUpdateOperationCreate,

			assertLog: noLogAssert,
		},
		{
			name:           "skip - rename - old does not exist",
			operation:      provisioning.ClusterUpdateOperationRename,
			clusterName:    "new",
			oldClusterName: "does_not_exist", // does not exist

			assertLog: noLogAssert,
		},
		{
			name:           "error - rename - new does already exist",
			operation:      provisioning.ClusterUpdateOperationRename,
			clusterName:    existingClusterName,
			oldClusterName: clusterName,

			assertLog: logContains("Failed to rename cluster artifact storage directory"),
		},
		{
			name:        "skip - delete - does not exist",
			operation:   provisioning.ClusterUpdateOperationDelete,
			clusterName: "does_not_exist", // does not exist

			assertLog: noLogAssert,
		},
	}

	_ = logContains

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			err := os.Mkdir(filepath.Join(tmpDir, clusterName), 0o700)
			require.NoError(t, err)
			err = os.Mkdir(filepath.Join(tmpDir, existingClusterName), 0o700)
			require.NoError(t, err)

			logBuf := &bytes.Buffer{}
			err = logger.InitLogger(logBuf, "", false, false)
			require.NoError(t, err)

			updateSignal := signals.NewSync[provisioning.ClusterUpdateMessage]()

			_, err = localartifact.New(nil, tmpDir, updateSignal)
			require.NoError(t, err)

			// Run test
			updateSignal.Emit(t.Context(), provisioning.ClusterUpdateMessage{
				Operation: tc.operation,
				Name:      tc.clusterName,
				OldName:   tc.oldClusterName,
			})

			// Assert
			tc.assertLog(t, logBuf)
			t.Log(logBuf.String())
		})
	}
}

func TestLocalArtifactDatabaseActions(t *testing.T) {
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
	clusterEntities.PreparedStmts, err = clusterEntities.PrepareStmts(tx, false)
	require.NoError(t, err)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	cluster := sqlite.NewCluster(tx)

	// Add cluster
	_, err = cluster.Create(ctx, clusterA)
	require.NoError(t, err)

	// Add cluster artifact from path
	artifactRepo, err := localartifact.New(tx, filepath.Join(tmpDir, "artifacts"), signals.NewSync[provisioning.ClusterUpdateMessage]())
	require.NoError(t, err)

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
	n, err := io.Copy(&buf, rc)
	require.NoError(t, err)
	require.Equal(t, int64(size), n)

	zipFile := memfile.New(buf.Bytes())

	zr, err := zip.NewReader(zipFile, int64(size))
	require.NoError(t, err)

	expectedFilesFound := map[string]bool{
		"one.txt": false,
		"two.txt": false,
	}

	for _, file := range zr.File {
		found, ok := expectedFilesFound[file.Name]
		require.True(t, ok, "unexpected file %q found in zip archive", file.Name)
		require.False(t, found, "file %q has already been seen", file.Name)

		expectedFilesFound[file.Name] = true
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
