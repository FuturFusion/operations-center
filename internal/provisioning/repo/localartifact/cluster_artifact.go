package localartifact

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/gabriel-vasile/mimetype"
	"github.com/lxc/incus/v7/shared/revert"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

type clusterArtifact struct {
	db   sqlite.DBTX
	root *os.Root
}

var _ provisioning.ClusterArtifactRepo = &clusterArtifact{}

// New returns a cluster artifact repository, which stores the files in root.
func New(db sqlite.DBTX, root *os.Root) *clusterArtifact {
	c := &clusterArtifact{
		db:   db,
		root: root,
	}

	c.registerUpdateSignalHandler()

	return c
}

// artifactPath joins elements to a path relative to the root. Each element
// must be a single file name, so the path cannot leave the root.
func artifactPath(elements ...string) (string, error) {
	for _, element := range elements {
		if !file.IsPathElement(element) {
			return "", domain.NewValidationErrf("Invalid cluster artifact, %q is not a valid file name", element)
		}
	}

	return filepath.Join(elements...), nil
}

func (c clusterArtifact) registerUpdateSignalHandler() {
	lifecycle.ClusterUpdateSignal.AddListener(func(ctx context.Context, cum lifecycle.ClusterUpdateMessage) {
		switch cum.Operation {
		case lifecycle.ClusterUpdateOperationRename:
			oldPath, err := artifactPath(cum.OldName)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to rename cluster artifact storage directory", slog.String("old_name", cum.OldName), logger.Err(err))
				return
			}

			newPath, err := artifactPath(cum.Name)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to rename cluster artifact storage directory", slog.String("new_name", cum.Name), logger.Err(err))
				return
			}

			if !file.PathExistsIn(c.root, oldPath) {
				return
			}

			err = c.root.Rename(oldPath, newPath)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to rename cluster artifact storage directory", slog.String("old_path", oldPath), slog.String("new_path", newPath), logger.Err(err))
				return
			}

		case lifecycle.ClusterUpdateOperationDelete:
			configDir, err := artifactPath(cum.Name)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to remove cluster artifact storage directory", slog.String("cluster", cum.Name), logger.Err(err))
				return
			}

			if !file.PathExistsIn(c.root, configDir) {
				return
			}

			err = c.root.RemoveAll(configDir)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to remove cluster artifact storage directory", slog.String("cluster", cum.Name), logger.Err(err))
			}

		default:
		}
	})
}

// CreateClusterArtifactFromPath creates a cluster artifact from the files in
// path. Path is either a directory or a single file. It can be outside of the
// root, for example a temporary directory.
func (c clusterArtifact) CreateClusterArtifactFromPath(ctx context.Context, in provisioning.ClusterArtifact, path string, ignoredFiles []string) (int64, error) {
	if in.Cluster == "" {
		return 0, domain.NewErrorf(domain.ErrConstraintViolation, "", "Cluster name cannot be empty")
	}

	if in.Name == "" {
		return 0, domain.NewErrorf(domain.ErrConstraintViolation, "", "Cluster artifact name cannot be empty")
	}

	targetDir, err := artifactPath(in.Cluster, in.Name)
	if err != nil {
		return 0, err
	}

	sourceRoot, sourceFilename, err := openSourceRoot(path)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path %q: %w", path, err)
	}

	defer sourceRoot.Close()

	in.Files = nil

	if sourceFilename == "" {
		dirEntries, err := fs.ReadDir(sourceRoot.FS(), ".")
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
		fi, err := sourceRoot.Stat(sourceFilename)
		if err != nil {
			return 0, fmt.Errorf("Failed to create cluster artifact from path %q: %w", path, err)
		}

		in.Files = append(in.Files, provisioning.ClusterArtifactFile{
			Name: sourceFilename,
			Size: fi.Size(),
		})
	}

	for i := range in.Files {
		mtype, err := detectMimeType(sourceRoot, in.Files[i].Name)
		if err != nil {
			return 0, fmt.Errorf("Failed to create cluster artifact from path %q: Failed to detect mimetype for %q: %w", path, in.Files[i].Name, err)
		}

		in.Files[i].MimeType = mtype.String()
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		_ = c.root.RemoveAll(targetDir)
	})

	err = c.root.MkdirAll(targetDir, 0o700)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	for _, f := range in.Files {
		_, err = copyFile(sourceRoot, f.Name, c.root, filepath.Join(targetDir, f.Name))
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

// openSourceRoot opens path as root, if path is a directory. If path is a
// file, it opens the parent directory as root and returns the file name.
func openSourceRoot(path string) (_ *os.Root, filename string, _ error) {
	parent, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, "", err
	}

	base := filepath.Base(path)

	fi, err := parent.Stat(base)
	if err != nil {
		parent.Close()
		return nil, "", err
	}

	if !fi.IsDir() {
		return parent, base, nil
	}

	defer parent.Close()

	root, err := parent.OpenRoot(base)
	if err != nil {
		return nil, "", err
	}

	return root, "", nil
}

func detectMimeType(root *os.Root, name string) (*mimetype.MIME, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	return mimetype.DetectReader(f)
}

func copyFile(srcRoot *os.Root, src string, dstRoot *os.Root, dst string) (_ int64, err error) {
	source, err := srcRoot.Open(src)
	if err != nil {
		return 0, err
	}

	defer source.Close()

	destination, err := dstRoot.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}

	defer func() {
		err = errors.Join(err, destination.Close())
	}()

	nBytes, err := file.SafeCopy(destination, source)
	return nBytes, err
}

// openFunc returns a function, which opens the file of the artifact.
func (c clusterArtifact) openFunc(clusterName string, artifactName string, filename string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		name, err := artifactPath(clusterName, artifactName, filename)
		if err != nil {
			return nil, err
		}

		return c.root.Open(name)
	}
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
			artifacts[i].Files[j].Open = c.openFunc(artifacts[i].Cluster, artifacts[i].Name, artifacts[i].Files[j].Name)
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
		artifact.Files[i].Open = c.openFunc(artifact.Cluster, artifact.Name, artifact.Files[i].Name)
	}

	return artifact, nil
}

func (c clusterArtifact) GetClusterArtifactArchiveByName(ctx context.Context, clusterName string, artifactName string, archiveType provisioning.ClusterArtifactArchiveType) (_ io.ReadCloser, size int, _ error) {
	if archiveType.Ext != provisioning.ClusterArtifactArchiveTypeExtZip {
		return nil, 0, fmt.Errorf("Archive type %q (%s) not supported", archiveType.Ext, archiveType.MimeType)
	}

	sourceDir, err := artifactPath(clusterName, artifactName)
	if err != nil {
		return nil, 0, err
	}

	if !file.PathExistsIn(c.root, sourceDir) {
		return nil, 0, fmt.Errorf("Files for artifact %q of cluster %q not found", artifactName, clusterName)
	}

	buf := new(bytes.Buffer)

	zipWriter := zip.NewWriter(buf)

	dirEntries, err := fs.ReadDir(c.root.FS(), sourceDir)
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
		err = copyToZip(zipFileWriter, c.root, sourceFilename)
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

func copyToZip(w io.Writer, root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return fmt.Errorf("Failed to open source file %q: %w", name, err)
	}

	defer f.Close()

	_, err = file.SafeCopy(w, f)

	return err
}
