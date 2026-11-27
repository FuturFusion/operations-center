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
	db      sqlite.DBTX
	root    *os.Root
	targets memberfiles.Targets
}

var (
	_ provisioning.ClusterArtifactRepo = &clusterArtifact{}
	_ memberfiles.FileTargetRepo       = &clusterArtifact{}
)

// New returns the artifacts stored in root by this member, targets resolves the other members.
func New(db sqlite.DBTX, root *os.Root, targets memberfiles.Targets) *clusterArtifact {
	c := &clusterArtifact{
		db:      db,
		root:    root,
		targets: targets,
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
		if cum.Operation != lifecycle.ClusterUpdateOperationDelete || cum.UUID == uuid.Nil {
			return
		}

		err := c.root.RemoveAll(cum.UUID.String())
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

	_, err := artifactPath(in.Name)
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

	// The files are copied outside of a transaction, so only the statements are run in one.
	clusterUUID, err := transaction.DoAndReturn(ctx, func(ctx context.Context) (string, error) {
		var clusterUUID string
		err := transaction.GetDBTX(ctx, c.db).QueryRowContext(ctx, `SELECT uuid FROM clusters WHERE name = ?`, in.Cluster).Scan(&clusterUUID)

		return clusterUUID, err
	})
	if err != nil {
		return 0, fmt.Errorf("Failed to get UUID of cluster %q: %w", in.Cluster, err)
	}

	in.MemberID, err = c.targets.LocalID()
	if err != nil {
		return 0, err
	}

	targetDir, err := artifactPath(clusterUUID, in.Name)
	if err != nil {
		return 0, err
	}

	err = c.root.MkdirAll(clusterUUID, 0o700)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	// Copy the files next to the target, so the files of an existing artifact are never touched.
	tmpDir := filepath.Join(clusterUUID, "."+in.Name+".partial-"+uuid.NewString())
	err = c.root.Mkdir(tmpDir, 0o700)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	defer func() {
		_ = c.root.RemoveAll(tmpDir)
	}()

	for _, f := range in.Files {
		_, err = copyFile(sourceRoot, f.Name, c.root, filepath.Join(tmpDir, f.Name))
		if err != nil {
			return 0, fmt.Errorf("Failed to create cluster artifact from path %q: Failed to copy file %q: %w", path, f.Name, err)
		}
	}

	// Renaming fails if the target exists already, so it is only removed if created here.
	err = c.root.Rename(tmpDir, targetDir)
	if err != nil {
		return 0, fmt.Errorf("Failed to create cluster artifact from path: %w", err)
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		_ = c.root.RemoveAll(targetDir)
	})

	id, err := transaction.DoAndReturn(ctx, func(ctx context.Context) (int64, error) {
		return entities.CreateClusterArtifact(ctx, transaction.GetDBTX(ctx, c.db), in)
	})
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

// filePath returns the slash separated path of the file filename of artifact in the store.
func filePath(artifact provisioning.ClusterArtifact, filename string) string {
	return filepath.ToSlash(filepath.Join(artifact.ClusterUUID.String(), artifact.Name, filename))
}

// open returns a function to open the file filename of artifact.
func (c clusterArtifact) open(artifact provisioning.ClusterArtifact, filename string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		name, err := artifactPath(artifact.ClusterUUID.String(), artifact.Name, filename)
		if err != nil {
			return nil, err
		}

		return c.root.Open(name)
	}
}

func (c clusterArtifact) GetClusterArtifactAll(ctx context.Context, clusterName string) (provisioning.ClusterArtifacts, error) {
	return transaction.DoAndReturn(ctx, func(ctx context.Context) (provisioning.ClusterArtifacts, error) {
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
	})
}

func (c clusterArtifact) GetClusterArtifactAllNames(ctx context.Context, clusterName string) ([]string, error) {
	return transaction.DoAndReturn(ctx, func(ctx context.Context) ([]string, error) {
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
	})
}

func (c clusterArtifact) GetClusterArtifactByName(ctx context.Context, clusterName string, artifactName string) (*provisioning.ClusterArtifact, error) {
	return transaction.DoAndReturn(ctx, func(ctx context.Context) (*provisioning.ClusterArtifact, error) {
		artifact, err := entities.GetClusterArtifact(ctx, transaction.GetDBTX(ctx, c.db), clusterName, artifactName)
		if err != nil {
			return nil, err
		}

		for i := range artifact.Files {
			artifact.Files[i].Open = c.open(*artifact, artifact.Files[i].Name)
		}

		return artifact, nil
	})
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

// Prune removes the artifact files the database does not assign to this member.
func (c clusterArtifact) Prune(ctx context.Context) error {
	localMemberID, err := c.targets.LocalID()
	if err != nil {
		return err
	}

	// The unknown files are removed outside of a transaction.
	artifacts, err := transaction.DoAndReturn(ctx, func(ctx context.Context) ([]provisioning.ClusterArtifact, error) {
		return entities.GetClusterArtifacts(ctx, transaction.GetDBTX(ctx, c.db))
	})
	if err != nil {
		return err
	}

	known := map[string]bool{}
	for _, artifact := range artifacts {
		if artifact.MemberID != localMemberID {
			continue
		}

		for _, artifactFile := range artifact.Files {
			known[filePath(artifact, artifactFile.Name)] = true
		}
	}

	return memberfiles.DeleteUnknownFiles(c.root, known)
}

// MigrateNameLayout moves the files, which are stored by cluster name, to the
// directory named by the UUID of the cluster.
func (c clusterArtifact) MigrateNameLayout(ctx context.Context) error {
	// The directories are renamed outside of a transaction.
	clusterUUIDs, err := transaction.DoAndReturn(ctx, func(ctx context.Context) (map[string]string, error) {
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

		return clusterUUIDs, err
	})
	if err != nil {
		return err
	}

	for name, clusterUUID := range clusterUUIDs {
		// A cluster name that is not a single file name has never been used as directory.
		if !file.IsPathElement(name) {
			continue
		}

		if !file.PathExistsIn(c.root, name) || file.PathExistsIn(c.root, clusterUUID) {
			continue
		}

		err = c.root.Rename(name, clusterUUID)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetUnitsByTarget returns the files of the cluster artifacts stored on the member memberID.
func (c clusterArtifact) GetUnitsByTarget(ctx context.Context, memberID int64) ([]memberfiles.Unit, error) {
	return transaction.DoAndReturn(ctx, func(ctx context.Context) ([]memberfiles.Unit, error) {
		var units []memberfiles.Unit

		err := query.Scan(ctx, transaction.GetDBTX(ctx, c.db), `
SELECT cluster_artifacts.id, clusters.uuid, cluster_artifacts.name
  FROM cluster_artifacts
  JOIN clusters ON clusters.id = cluster_artifacts.cluster_id
  WHERE cluster_artifacts.member_id = ?`, func(scan func(dest ...any) error) error {
			var unit memberfiles.Unit
			var clusterUUID, name string

			err := scan(&unit.ID, &clusterUUID, &name)
			if err != nil {
				return err
			}

			unit.Store = memberfiles.StoreArtifacts
			unit.Name = filepath.ToSlash(filepath.Join(clusterUUID, name))
			units = append(units, unit)

			return nil
		}, memberID)
		if err != nil {
			return nil, err
		}

		return units, nil
	})
}

// SetUnitTarget records that the member memberID stores the files of the cluster artifact unit.
func (c clusterArtifact) SetUnitTarget(ctx context.Context, unit memberfiles.Unit, memberID int64) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		_, err := transaction.GetDBTX(ctx, c.db).ExecContext(ctx, `UPDATE cluster_artifacts SET member_id = ? WHERE id = ?`, memberID, unit.ID)
		return err
	})
}
