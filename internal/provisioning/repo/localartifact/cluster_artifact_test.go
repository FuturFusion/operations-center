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

	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	clusterEntities "github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestLocalArtifact_renameAndDelete(t *testing.T) {
	tx, cluster, sourcePath := setupArtifactTest(t)

	oldClusterUpdateSignal := lifecycle.ClusterUpdateSignal
	lifecycle.ClusterUpdateSignal = signals.NewSync[lifecycle.ClusterUpdateMessage]()
	t.Cleanup(func() {
		lifecycle.ClusterUpdateSignal = oldClusterUpdateSignal
	})

	storageDir := filepath.Join(t.TempDir(), "artifacts")
	artifactRepo, err := localartifact.New(tx, storageDir, standalone)
	require.NoError(t, err)

	_, err = artifactRepo.CreateClusterArtifactFromPath(t.Context(), provisioning.ClusterArtifact{Cluster: "clusterOne", Name: "one"}, sourcePath, nil)
	require.NoError(t, err)

	dbCluster, err := cluster.GetByName(t.Context(), "clusterOne")
	require.NoError(t, err)

	clusterDir := filepath.Join(storageDir, dbCluster.UUID.String())
	require.FileExists(t, filepath.Join(clusterDir, "one", "one.txt"))

	// A rename does not touch the files.
	err = cluster.Rename(t.Context(), "clusterOne", "clusterTwo")
	require.NoError(t, err)

	artifact, err := artifactRepo.GetClusterArtifactByName(t.Context(), "clusterTwo", "one")
	require.NoError(t, err)

	r, err := artifact.Files[0].Open()
	require.NoError(t, err)
	require.NoError(t, r.Close())

	err = cluster.DeleteByName(t.Context(), "clusterTwo")
	require.NoError(t, err)

	lifecycle.ClusterUpdateSignal.Emit(t.Context(), lifecycle.ClusterUpdateMessage{
		Operation: lifecycle.ClusterUpdateOperationDelete,
		Name:      "clusterTwo",
		UUID:      dbCluster.UUID,
	})

	require.NoDirExists(t, clusterDir)
}

func TestLocalArtifact_createExisting(t *testing.T) {
	tx, cluster, sourcePath := setupArtifactTest(t)

	storageDir := filepath.Join(t.TempDir(), "artifacts")
	artifactRepo, err := localartifact.New(tx, storageDir, standalone)
	require.NoError(t, err)

	_, err = artifactRepo.CreateClusterArtifactFromPath(t.Context(), provisioning.ClusterArtifact{Cluster: "clusterOne", Name: "one"}, sourcePath, nil)
	require.NoError(t, err)

	dbCluster, err := cluster.GetByName(t.Context(), "clusterOne")
	require.NoError(t, err)

	filename := filepath.Join(storageDir, dbCluster.UUID.String(), "one", "one.txt")
	want, err := os.ReadFile(filename)
	require.NoError(t, err)

	otherPath := t.TempDir()
	err = os.WriteFile(filepath.Join(otherPath, "one.txt"), []byte("other"), 0o600)
	require.NoError(t, err)

	// An artifact of the same name is refused without touching the files of the existing one.
	_, err = artifactRepo.CreateClusterArtifactFromPath(t.Context(), provisioning.ClusterArtifact{Cluster: "clusterOne", Name: "one"}, otherPath, nil)
	require.Error(t, err)

	got, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLocalArtifact_MigrateNameLayout(t *testing.T) {
	tx, cluster, sourcePath := setupArtifactTest(t)

	storageDir := filepath.Join(t.TempDir(), "artifacts")
	artifactRepo, err := localartifact.New(tx, storageDir, standalone)
	require.NoError(t, err)

	_, err = artifactRepo.CreateClusterArtifactFromPath(t.Context(), provisioning.ClusterArtifact{Cluster: "clusterOne", Name: "one"}, sourcePath, nil)
	require.NoError(t, err)

	dbCluster, err := cluster.GetByName(t.Context(), "clusterOne")
	require.NoError(t, err)

	// Move the files to the directory named by the cluster.
	err = os.Rename(filepath.Join(storageDir, dbCluster.UUID.String()), filepath.Join(storageDir, "clusterOne"))
	require.NoError(t, err)

	err = artifactRepo.MigrateNameLayout(t.Context())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(storageDir, dbCluster.UUID.String(), "one", "one.txt"))
	require.NoDirExists(t, filepath.Join(storageDir, "clusterOne"))
}

// standalone resolves no other cluster members.
var standalone = memberfiles.NewTargets(func() memberfiles.Cluster { return nil })

// setupArtifactTest returns a database with the cluster "clusterOne" and a
// source directory with the files "one.txt" and "two.txt".
func setupArtifactTest(t *testing.T) (transaction.DBTX, provisioning.ClusterRepo, string) {
	t.Helper()

	tmpDir := t.TempDir()

	sourcePath := filepath.Join(tmpDir, "source")
	err := os.Mkdir(sourcePath, 0o700)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(sourcePath, "one.txt"), []byte(`one`), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(sourcePath, "two.txt"), []byte(`two with more content`), 0o600)
	require.NoError(t, err)

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	tx := transaction.Enable(db)
	clusterEntities.PreparedStmts, err = clusterEntities.PrepareStmts(tx.(transaction.Preparer), false)
	require.NoError(t, err)
	entities.PreparedStmts, err = entities.PrepareStmts(tx.(transaction.Preparer), false)
	require.NoError(t, err)

	cluster := sqlite.NewCluster(tx)

	_, err = cluster.Create(t.Context(), provisioning.Cluster{
		Name:    "clusterOne",
		Channel: "stable",
	})
	require.NoError(t, err)

	return tx, cluster, sourcePath
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
	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	tx := transaction.Enable(db)
	clusterStmts, err := clusterEntities.PrepareStmts(tx.(transaction.Preparer), false)
	require.NoError(t, err)

	artifactStmts, err := entities.PrepareStmts(tx.(transaction.Preparer), false)
	require.NoError(t, err)

	ctx = clusterEntities.WithPreparedStmts(ctx, clusterStmts)
	ctx = entities.WithPreparedStmts(ctx, artifactStmts)

	cluster := sqlite.NewCluster(tx)

	// Add cluster
	_, err = cluster.Create(ctx, clusterA)
	require.NoError(t, err)

	// Add cluster artifact from path
	artifactRepo, err := localartifact.New(tx, filepath.Join(tmpDir, "artifacts"), standalone)
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
