package localartifact

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/gabriel-vasile/mimetype"
	"github.com/lxc/incus/v6/shared/revert"
	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

type clusterArtifact struct {
	db         sqlite.DBTX
	storageDir string
}

var _ provisioning.ClusterArtifactRepo = &clusterArtifact{}

func New(db sqlite.DBTX, storageDir string, updateSignal signals.Signal[provisioning.ClusterUpdateMessage]) (*clusterArtifact, error) {
	err := os.MkdirAll(storageDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory for local artifact storage: %w", err)
	}

	c := &clusterArtifact{
		db:         db,
		storageDir: storageDir,
	}

	c.registerUpdateSignalHandler(updateSignal)

	return c, nil
}

func (c clusterArtifact) registerUpdateSignalHandler(clusterUpdateSignal signals.Signal[provisioning.ClusterUpdateMessage]) {
	clusterUpdateSignal.AddListener(func(ctx context.Context, cum provisioning.ClusterUpdateMessage) {
		switch cum.Operation {
		case provisioning.ClusterUpdateOperationRename:
			oldPath := filepath.Join(c.storageDir, cum.OldName)
			newPath := filepath.Join(c.storageDir, cum.Name)
			if !file.PathExists(oldPath) {
				return
			}

			err := os.Rename(oldPath, newPath)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to rename cluster artifact storage directory", slog.String("old_path", oldPath), slog.String("new_path", newPath), logger.Err(err))
				return
			}

		case provisioning.ClusterUpdateOperationDelete:
			configDir := filepath.Join(c.storageDir, cum.Name)
			if !file.PathExists(configDir) {
				return
			}

			err := os.RemoveAll(configDir)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to remove cluster artifact storage directory", slog.String("cluster", cum.Name), logger.Err(err))
			}

		default:
		}
	})
}

func (c clusterArtifact) CreateClusterArtifactFromPath(ctx context.Context, in provisioning.ClusterArtifact, path string, ignoredFiles []string) (int64, error) {
	if in.Cluster == "" {
		return 0, fmt.Errorf("Failed to create cluster artifact from path, cluster name can not be empty: %w", domain.ErrConstraintViolation)
	}

	if in.Name == "" {
		return 0, fmt.Errorf("Failed to create cluster artifact from path, artifact name can not be empty: %w", domain.ErrConstraintViolation)
	}

	if !file.PathExists(path) {
		return 0, fmt.Errorf("Failed to create cluster artifact from path %q: Path does not exist", path)
	}

	fi, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path %q: %w", path, err)
	}

	in.Files = nil

	if fi.IsDir() {
		dirEntries, err := os.ReadDir(path)
		if err != nil {
			return 0, fmt.Errorf("Failed to create cluster artifact from path %q: %w", path, err)
		}

		for _, entry := range dirEntries {
			if entry.IsDir() || !entry.Type().IsRegular() {
				// Only regular files are added to the artifact.
				continue
			}

			if slices.Contains(ignoredFiles, entry.Name()) {
				continue
			}

			entryFI, err := entry.Info()
			if err != nil {
				return 0, fmt.Errorf("Failed to create cluster artifact from path %q: Failed to get File info for %q: %w", path, entry.Name(), err)
			}

			in.Files = append(in.Files, provisioning.ClusterArtifactFile{
				Name: entry.Name(),
				Size: entryFI.Size(),
			})
		}
	} else {
		in.Files = append(in.Files, provisioning.ClusterArtifactFile{
			Name: filepath.Base(path),
			Size: fi.Size(),
		})

		path = filepath.Dir(path)
	}

	for i := range in.Files {
		mtype, err := mimetype.DetectFile(filepath.Join(path, in.Files[i].Name))
		if err != nil {
			return 0, fmt.Errorf("Failed to create cluster artifact from path %q: Failed to detect mimetype for %q: %w", path, in.Files[i].Name, err)
		}

		in.Files[i].MimeType = mtype.String()
	}

	targetDir := filepath.Join(c.storageDir, in.Cluster, in.Name)

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		_ = os.RemoveAll(targetDir)
	})

	err = os.MkdirAll(targetDir, 0o700)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	for _, f := range in.Files {
		_, err = copyFile(filepath.Join(path, f.Name), filepath.Join(targetDir, f.Name))
		if err != nil {
			return 0, fmt.Errorf("Failed to create cluster artifact from path %q: Failed to copy file %q: %w", path, f.Name, err)
		}
	}

	id, err := entities.CreateClusterArtifact(ctx, transaction.GetDBTX(ctx, c.db), in)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact entry: %w", err)
	}

	reverter.Success()

	return id, nil
}

func copyFile(src string, dst string) (_ int64, err error) {
	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}

	defer source.Close()

	destination, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}

	defer func() {
		err = errors.Join(err, destination.Close())
	}()

	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

func (c clusterArtifact) GetClusterArtifactAll(ctx context.Context, clusterName string) (provisioning.ClusterArtifacts, error) {
	artifacts, err := entities.GetClusterArtifacts(ctx, transaction.GetDBTX(ctx, c.db), entities.ClusterArtifactFilter{
		Cluster: &clusterName,
	})
	if err != nil {
		return nil, err
	}

	for i := range artifacts {
		for j := range artifacts[i].Files {
			artifacts[i].Files[j].Open = func() (io.ReadCloser, error) {
				return os.Open(filepath.Join(c.storageDir, artifacts[i].Cluster, artifacts[i].Name, artifacts[i].Files[j].Name))
			}
		}
	}

	return artifacts, nil
}

func (c clusterArtifact) GetClusterArtifactAllNames(ctx context.Context, clusterName string) ([]string, error) {
	artifacts, err := entities.GetClusterArtifacts(ctx, transaction.GetDBTX(ctx, c.db), entities.ClusterArtifactFilter{
		Cluster: &clusterName,
	})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		names = append(names, artifact.Name)
	}

	return names, nil
}

func (c clusterArtifact) GetClusterArtifactByName(ctx context.Context, clusterName string, artifactName string) (*provisioning.ClusterArtifact, error) {
	artifact, err := entities.GetClusterArtifact(ctx, transaction.GetDBTX(ctx, c.db), clusterName, artifactName)
	if err != nil {
		return nil, err
	}

	for i := range artifact.Files {
		artifact.Files[i].Open = func() (io.ReadCloser, error) {
			return os.Open(filepath.Join(c.storageDir, artifact.Cluster, artifact.Name, artifact.Files[i].Name))
		}
	}

	return artifact, nil
}

func (c clusterArtifact) GetClusterArtifactArchiveByName(ctx context.Context, clusterName string, artifactName string, archiveType provisioning.ClusterArtifactArchiveType) (_ io.ReadCloser, size int, _ error) {
	if archiveType.Ext != provisioning.ClusterArtifactArchiveTypeExtZip {
		return nil, 0, fmt.Errorf("Archive type %q (%s) not supported", archiveType.Ext, archiveType.MimeType)
	}

	sourceDir := filepath.Join(c.storageDir, clusterName, artifactName)
	if !file.PathExists(sourceDir) {
		return nil, 0, fmt.Errorf("Files for artifact %q of cluster %q not found", artifactName, clusterName)
	}

	buf := new(bytes.Buffer)

	zipWriter := zip.NewWriter(buf)

	dirEntries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to read directory entries for %q: %w", sourceDir, err)
	}

	for _, f := range dirEntries {
		if f.IsDir() || !f.Type().IsRegular() {
			continue
		}

		zipFileWriter, err := zipWriter.Create(f.Name())
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to create %q in zip file: %w", f.Name(), err)
		}

		sourceFilename := filepath.Join(sourceDir, f.Name())
		sourceFile, err := os.Open(sourceFilename)
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to open source file %q: %w", sourceFilename, err)
		}

		_, err = io.Copy(zipFileWriter, sourceFile)
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to copy content from source file %q to zip archive: %w", sourceFilename, err)
		}
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to close zip archive: %w", err)
	}

	return io.NopCloser(buf), buf.Len(), nil
}
