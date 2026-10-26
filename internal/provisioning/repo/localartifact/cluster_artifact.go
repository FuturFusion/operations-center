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
	"github.com/google/uuid"
	"github.com/lxc/incus/v7/shared/revert"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localartifact/entities"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite/query"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// clusterArtifact stores the artifact files of a cluster in a directory named by the cluster UUID.
type clusterArtifact struct {
	db         sqlite.DBTX
	storageDir string
	targets    memberfiles.Targets
}

var _ provisioning.ClusterArtifactRepo = &clusterArtifact{}

// New returns the artifacts stored in storageDir by this member, targets resolves the other members.
func New(db sqlite.DBTX, storageDir string, targets memberfiles.Targets) (*clusterArtifact, error) {
	err := os.MkdirAll(storageDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory for local artifact storage: %w", err)
	}

	c := &clusterArtifact{
		db:         db,
		storageDir: storageDir,
		targets:    targets,
	}

	c.registerUpdateSignalHandler()

	return c, nil
}

func (c clusterArtifact) registerUpdateSignalHandler() {
	lifecycle.ClusterUpdateSignal.AddListener(func(ctx context.Context, cum lifecycle.ClusterUpdateMessage) {
		if cum.Operation != lifecycle.ClusterUpdateOperationDelete || cum.UUID == uuid.Nil {
			return
		}

		err := os.RemoveAll(filepath.Join(c.storageDir, cum.UUID.String()))
		if err != nil {
			slog.ErrorContext(ctx, "Failed to remove cluster artifact storage directory", slog.String("cluster", cum.Name), logger.Err(err))
		}

		// The artifacts might be stored by another member, unreachable members remove the files on startup.
		members, err := c.targets.Others(ctx)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get the cluster members to remove the cluster artifacts on", slog.String("cluster", cum.Name), logger.Err(err))
			return
		}

		for _, member := range members {
			err := c.targets.Do(ctx, member.ID, func(ctx context.Context, cl client.OperationsCenterClient) error {
				return cl.DeleteMemberFile(ctx, memberfiles.StoreArtifacts, cum.UUID.String())
			})
			if err != nil {
				slog.WarnContext(ctx, "Failed to remove the cluster artifacts on cluster member", slog.String("cluster", cum.Name), slog.String("member", member.Name), logger.Err(err))
			}
		}
	})
}

func (c clusterArtifact) CreateClusterArtifactFromPath(ctx context.Context, in provisioning.ClusterArtifact, path string, ignoredFiles []string) (int64, error) {
	if in.Cluster == "" {
		return 0, domain.NewErrorf(domain.ErrConstraintViolation, "", "Cluster name cannot be empty")
	}

	if in.Name == "" {
		return 0, domain.NewErrorf(domain.ErrConstraintViolation, "", "Cluster artifact name cannot be empty")
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

	var clusterUUID string
	err = transaction.GetDBTX(ctx, c.db).QueryRowContext(ctx, `SELECT uuid FROM clusters WHERE name = ?`, in.Cluster).Scan(&clusterUUID)
	if err != nil {
		return 0, fmt.Errorf("Failed to get UUID of cluster %q: %w", in.Cluster, err)
	}

	in.MemberID, err = c.targets.LocalID()
	if err != nil {
		return 0, err
	}

	targetDir := filepath.Join(c.storageDir, clusterUUID, in.Name)

	err = os.MkdirAll(filepath.Dir(targetDir), 0o700)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	// Copy the files next to the target, so the files of an existing artifact are never touched.
	tmpDir, err := os.MkdirTemp(filepath.Dir(targetDir), "."+in.Name+".partial-*")
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	for _, f := range in.Files {
		_, err = copyFile(filepath.Join(path, f.Name), filepath.Join(tmpDir, f.Name))
		if err != nil {
			return 0, fmt.Errorf("Failed to create cluster artifact from path %q: Failed to copy file %q: %w", path, f.Name, err)
		}
	}

	// Renaming fails if the target exists already, so it is only removed if created here.
	err = os.Rename(tmpDir, targetDir)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		_ = os.RemoveAll(targetDir)
	})

	id, err := entities.CreateClusterArtifact(ctx, transaction.GetDBTX(ctx, c.db), in)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact entry: %w", err)
	}

	reverter.Success()

	return id, nil
}

// filePath returns the slash separated path of the file filename of artifact in the store.
func filePath(artifact provisioning.ClusterArtifact, filename string) string {
	return filepath.ToSlash(filepath.Join(artifact.ClusterUUID.String(), artifact.Name, filename))
}

// open returns a function to open the file filename of artifact.
func (c clusterArtifact) open(artifact provisioning.ClusterArtifact, filename string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		return os.Open(filepath.Join(c.storageDir, filepath.FromSlash(filePath(artifact, filename))))
	}
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

	nBytes, err := file.SafeCopy(destination, source)
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
			artifacts[i].Files[j].Open = c.open(artifacts[i], artifacts[i].Files[j].Name)
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
		artifact.Files[i].Open = c.open(*artifact, artifact.Files[i].Name)
	}

	return artifact, nil
}

func (c clusterArtifact) GetClusterArtifactArchiveByName(ctx context.Context, clusterName string, artifactName string, archiveType provisioning.ClusterArtifactArchiveType) (_ io.ReadCloser, size int, _ error) {
	if archiveType.Ext != provisioning.ClusterArtifactArchiveTypeExtZip {
		return nil, 0, fmt.Errorf("Archive type %q (%s) not supported", archiveType.Ext, archiveType.MimeType)
	}

	artifact, err := c.GetClusterArtifactByName(ctx, clusterName, artifactName)
	if err != nil {
		return nil, 0, err
	}

	buf := new(bytes.Buffer)

	zipWriter := zip.NewWriter(buf)

	for _, f := range artifact.Files {
		zipFileWriter, err := zipWriter.Create(f.Name)
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to create %q in zip file: %w", f.Name, err)
		}

		err = copyToArchive(zipFileWriter, f)
		if err != nil {
			return nil, 0, err
		}
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to close zip archive: %w", err)
	}

	return io.NopCloser(buf), buf.Len(), nil
}

func copyToArchive(w io.Writer, f provisioning.ClusterArtifactFile) error {
	sourceFile, err := f.Open()
	if err != nil {
		return fmt.Errorf("Failed to open source file %q: %w", f.Name, err)
	}

	defer sourceFile.Close()

	_, err = file.SafeCopy(w, sourceFile)
	if err != nil {
		return fmt.Errorf("Failed to copy content from source file %q to zip archive: %w", f.Name, err)
	}

	return nil
}

// MigrateNameLayout moves the files, which are stored by cluster name, to the
// directory named by the UUID of the cluster.
func (c clusterArtifact) MigrateNameLayout(ctx context.Context) error {
	clusterUUIDs := map[string]string{}
	err := query.Scan(ctx, transaction.GetDBTX(ctx, c.db), `SELECT name, uuid FROM clusters`, func(scan func(dest ...any) error) error {
		var name, clusterUUID string

		err := scan(&name, &clusterUUID)
		if err != nil {
			return err
		}

		clusterUUIDs[name] = clusterUUID

		return nil
	})
	if err != nil {
		return err
	}

	for name, clusterUUID := range clusterUUIDs {
		nameDir := filepath.Join(c.storageDir, name)
		uuidDir := filepath.Join(c.storageDir, clusterUUID)
		if !file.PathExists(nameDir) || file.PathExists(uuidDir) {
			continue
		}

		err = os.Rename(nameDir, uuidDir)
		if err != nil {
			return err
		}
	}

	return nil
}
