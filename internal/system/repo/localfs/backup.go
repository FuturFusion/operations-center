package localfs

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lxc/incus/v7/shared/revert"
	"go.yaml.in/yaml/v4"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

// databaseFilename is the name of the database file in the var dir.
const databaseFilename = "local.db"

// backupDirName holds the database snapshots of backups in progress.
const backupDirName = ".backup"

// restoreDirName holds a staged restore, it must be on the var dir file system.
const restoreDirName = ".restore"

// restoreAppliedMarker marks a restore, which is in place but not yet completed.
const restoreAppliedMarker = "applied"

// backupManifestFilename is the name of the manifest, the first entry of a backup archive.
const backupManifestFilename = "backup.yaml"

// backupFormatVersion is raised, if older versions can not restore the backup anymore.
const backupFormatVersion = 1

// backupManifestMaxSize limits the size of the manifest read from a backup.
const backupManifestMaxSize = 1 << 20

// backupManifest describes a backup, only the format version decides, if it is accepted.
type backupManifest struct {
	FormatVersion int       `yaml:"format_version"`
	Version       string    `yaml:"operations_center_version"`
	CreatedAt     time.Time `yaml:"created_at"`
	WithImages    bool      `yaml:"with_images"`
	WithUpdates   bool      `yaml:"with_updates"`
}

// usageFunc returns the space usage of the file system a backup is extracted to.
type usageFunc func() (file.UsageInformation, error)

type backup struct {
	varDir string
	usage  usageFunc
}

var _ system.BackupRepo = backup{}

func (b backup) Create(ctx context.Context, options system.BackupOptions, snapshotDatabase func(ctx context.Context, dir string) error) (io.ReadCloser, error) {
	// Check upfront, errors can not be reported once the archive is streamed.
	for _, entry := range backupEntries {
		if !entry.required || entry.name == databaseFilename {
			continue
		}

		_, err := os.Stat(filepath.Join(b.varDir, entry.name))
		if err != nil {
			return nil, fmt.Errorf("Failed to access %q for backup: %w", entry.name, err)
		}
	}

	err := os.MkdirAll(filepath.Join(b.varDir, backupDirName), 0o700)
	if err != nil {
		return nil, err
	}

	snapshotDir, err := os.MkdirTemp(filepath.Join(b.varDir, backupDirName), "")
	if err != nil {
		return nil, err
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		err := os.RemoveAll(snapshotDir)
		if err != nil {
			slog.WarnContext(ctx, "Failed to remove database snapshot after unsuccessful backup", slog.String("directory", snapshotDir), logger.Err(err))
		}
	})

	err = snapshotDatabase(ctx, snapshotDir)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = pw.CloseWithError(writeBackup(pw, b.varDir, snapshotDir, options))
	}()

	reverter.Success()

	return &backupReader{
		PipeReader:  pr,
		done:        done,
		snapshotDir: snapshotDir,
	}, nil
}

// backupReader streams a backup and removes the database snapshot on Close.
type backupReader struct {
	*io.PipeReader

	done        chan struct{}
	snapshotDir string
}

func (b *backupReader) Close() error {
	_ = b.PipeReader.Close()
	<-b.done

	return os.RemoveAll(b.snapshotDir)
}

func (b backup) Extract(ctx context.Context, archive io.Reader) (_ string, err error) {
	restoreDir := filepath.Join(b.varDir, restoreDirName)
	pendingDir := filepath.Join(restoreDir, "pending")

	// The state replaced by an applied restore is needed to roll it back.
	if b.IsRestoreApplied() {
		return "", domain.NewErrorf(domain.ErrOperationNotPermitted, "", "A restore is already in progress")
	}

	// Leftovers of a previous upload, which has been interrupted.
	err = os.RemoveAll(pendingDir)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(pendingDir, 0o700)
	if err != nil {
		return "", err
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		discardErr := b.Discard(ctx)
		if discardErr != nil {
			slog.WarnContext(ctx, "Failed to remove restore directory after unsuccessful extraction", slog.String("directory", restoreDir), logger.Err(discardErr))
		}
	})

	err = extractBackup(ctx, archive, pendingDir, b.usage)
	if err != nil {
		return "", err
	}

	for _, entry := range backupEntries {
		if !entry.required {
			continue
		}

		_, err = os.Stat(filepath.Join(pendingDir, entry.name))
		if err != nil {
			return "", invalidBackupErr(err, "Backup is missing %q", entry.name)
		}
	}

	reverter.Success()

	return pendingDir, nil
}

func (b backup) Stage(_ context.Context) error {
	restoreDir := filepath.Join(b.varDir, restoreDirName)

	// The staged restore is moved in place by the next start.
	return os.Rename(filepath.Join(restoreDir, "pending"), filepath.Join(restoreDir, "staged"))
}

func (b backup) Discard(_ context.Context) error {
	return os.RemoveAll(filepath.Join(b.varDir, restoreDirName))
}

type backupEntry struct {
	name     string
	required bool

	// included reports, if an optional entry is part of a backup.
	included func(options system.BackupOptions) bool

	// keptIfAbsent keeps the entry present on the system, if the backup does not include it.
	keptIfAbsent bool
}

// backupEntries holds the var dir entries making up the state of Operations Center.
var backupEntries = []backupEntry{
	{name: config.ConfigFilename, required: true},
	{name: config.ServerCertificateFilename, required: true},
	{name: config.ServerKeyFilename, required: true},
	{name: config.ClientCertificateFilename, required: true},
	{name: config.ClientKeyFilename, required: true},
	{name: databaseFilename, required: true},
	{name: "artifacts"},
	{name: "images", included: func(options system.BackupOptions) bool { return options.Images }},
	{name: "updates", included: func(options system.BackupOptions) bool { return options.Updates }, keptIfAbsent: true},
}

func backupEntryNames() []string {
	names := make([]string, 0, len(backupEntries))
	for _, entry := range backupEntries {
		names = append(names, entry.name)
	}

	return names
}

// writeBackup writes the backup as gzip compressed tar archive to w.
func writeBackup(w io.Writer, varDir string, snapshotDir string, options system.BackupOptions) (err error) {
	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)

	err = writeManifest(tw, backupManifest{
		FormatVersion: backupFormatVersion,
		Version:       version.Version,
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		WithImages:    options.Images,
		WithUpdates:   options.Updates,
	})
	if err != nil {
		return err
	}

	for _, entry := range backupEntries {
		if entry.included != nil && !entry.included(options) {
			continue
		}

		root := varDir
		if entry.name == databaseFilename {
			root = snapshotDir
		}

		err = addToArchive(tw, root, entry.name)
		if err != nil {
			return err
		}
	}

	err = tw.Close()
	if err != nil {
		return fmt.Errorf("Failed to finish backup archive: %w", err)
	}

	err = gzw.Close()
	if err != nil {
		return fmt.Errorf("Failed to finish backup compression: %w", err)
	}

	return nil
}

// writeManifest writes the manifest to the archive, it has to be the first entry.
func writeManifest(tw *tar.Writer, manifest backupManifest) error {
	content, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("Failed to create backup manifest: %w", err)
	}

	err = tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     backupManifestFilename,
		Mode:     0o600,
		Size:     int64(len(content)),
		ModTime:  manifest.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("Failed to write archive header for %q: %w", backupManifestFilename, err)
	}

	_, err = tw.Write(content)
	if err != nil {
		return fmt.Errorf("Failed to add %q to archive: %w", backupManifestFilename, err)
	}

	return nil
}

// readManifest reads the manifest from the first entry of the archive and rejects unsupported backups.
func readManifest(tr *tar.Reader) (backupManifest, error) {
	header, err := tr.Next()
	if err != nil && !errors.Is(err, io.EOF) {
		return backupManifest{}, invalidBackupErr(err, "Failed to read backup archive")
	}

	if err != nil || header.Typeflag != tar.TypeReg || path.Clean(header.Name) != backupManifestFilename {
		return backupManifest{}, invalidBackupErr(nil, "Backup has no manifest")
	}

	if header.Size > backupManifestMaxSize {
		return backupManifest{}, invalidBackupErr(nil, "Backup manifest is too large")
	}

	content, err := io.ReadAll(tr)
	if err != nil {
		return backupManifest{}, invalidBackupErr(err, "Failed to read backup manifest")
	}

	var manifest backupManifest

	err = yaml.Unmarshal(content, &manifest)
	if err != nil || manifest.FormatVersion < 1 {
		return backupManifest{}, invalidBackupErr(err, "Backup has an invalid manifest")
	}

	if manifest.FormatVersion > backupFormatVersion {
		return backupManifest{}, domain.NewErrorf(domain.ErrInvalidArgument, "", "Backup format version %d is not supported, the latest supported version is %d", manifest.FormatVersion, backupFormatVersion).
			WithHintf("Update Operations Center before restoring this backup.")
	}

	return manifest, nil
}

func addToArchive(tw *tar.Writer, root string, name string) error {
	return filepath.WalkDir(filepath.Join(root, name), func(filename string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			// Optional entries, which do not exist, are skipped.
			if errors.Is(err, fs.ErrNotExist) && filename == filepath.Join(root, name) {
				return nil
			}

			return err
		}

		info, err := dirEntry.Info()
		if err != nil {
			return err
		}

		if !info.Mode().IsDir() && !info.Mode().IsRegular() {
			//domain-errors:internal Only Operations Center writes to its var dir.
			return fmt.Errorf("Unsupported file type of %q in backup", filename)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("Failed to create archive header for %q: %w", filename, err)
		}

		relName, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}

		header.Name = filepath.ToSlash(relName)
		if info.IsDir() {
			header.Name += "/"
		}

		err = tw.WriteHeader(header)
		if err != nil {
			return fmt.Errorf("Failed to write archive header for %q: %w", filename, err)
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(filename)
		if err != nil {
			return err
		}

		defer f.Close()

		_, err = io.Copy(tw, f)
		if err != nil {
			return fmt.Errorf("Failed to add %q to archive: %w", filename, err)
		}

		return nil
	})
}

// extractBackup extracts the known backup entries from r to dir.
func extractBackup(ctx context.Context, r io.Reader, dir string, usage usageFunc) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return invalidBackupErr(err, "Backup is not a gzip compressed archive")
	}

	defer func() {
		err := gzr.Close()
		if err != nil {
			slog.WarnContext(ctx, "Failed to close backup archive", logger.Err(err))
		}
	}()

	knownEntries := backupEntryNames()

	tr := tar.NewReader(gzr)

	// Unsupported backups are rejected, before anything is extracted.
	_, err = readManifest(tr)
	if err != nil {
		return err
	}

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			// Read up to the end, the checksum of the archive is only verified there.
			_, err = io.Copy(io.Discard, gzr)
			if err != nil {
				return invalidBackupErr(err, "Backup archive is corrupt")
			}

			return nil
		}

		if err != nil {
			return invalidBackupErr(err, "Failed to read backup archive")
		}

		name := path.Clean(header.Name)
		if !filepath.IsLocal(name) {
			return invalidBackupErr(nil, "Backup contains invalid path %q", header.Name)
		}

		topLevel, _, _ := strings.Cut(name, "/")
		if !slices.Contains(knownEntries, topLevel) {
			return invalidBackupErr(nil, "Backup contains unexpected entry %q", header.Name)
		}

		filename := filepath.Join(dir, filepath.FromSlash(name))
		mode := header.FileInfo().Mode().Perm()

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(filename, mode|0o700)
			if err != nil {
				return err
			}

		case tar.TypeReg:
			err = os.MkdirAll(filepath.Dir(filename), 0o700)
			if err != nil {
				return err
			}

			// The size of a backup is not known upfront, so each file is checked.
			err = checkSpaceAvailable(usage, header.Size)
			if err != nil {
				return err
			}

			err = extractFile(tr, filename, mode)
			if err != nil {
				return err
			}

		default:
			return invalidBackupErr(nil, "Backup entry %q has an unsupported type", header.Name)
		}
	}
}

// invalidBackupErr reports a backup, which can not be restored.
func invalidBackupErr(cause error, format string, a ...any) error {
	return domain.NewErrorf(domain.ErrInvalidArgument, "", format, a...).
		WithCause(cause).
		WithHintf("Provide a backup created by Operations Center.")
}

// checkSpaceAvailable ensures, that 10% of the total space is kept free after writing required bytes.
func checkSpaceAvailable(usage usageFunc, required int64) error {
	ui, err := usage()
	if err != nil {
		return fmt.Errorf("Failed to get usage information: %w", err)
	}

	if ui.TotalSpaceBytes < 1 {
		//domain-errors:internal Programmer error, the file system reports nonsense.
		return fmt.Errorf("File system reported an invalid total space: %d", ui.TotalSpaceBytes)
	}

	if (float64(ui.AvailableSpaceBytes)-float64(required))/float64(ui.TotalSpaceBytes) < 0.1 {
		return domain.NewErrorf(domain.ErrConstraintViolation, api.ErrorReasonInsufficientStorage, "Not enough space available to restore the backup, %d bytes are required, %d bytes are available and 10%% of the total space is kept free", required, ui.AvailableSpaceBytes).
			WithHintf("Free space in the data directory of Operations Center, e.g. by removing updates which are no longer needed.").
			WithDetail("required_bytes", strconv.FormatInt(required, 10)).
			WithDetail("available_bytes", strconv.FormatUint(ui.AvailableSpaceBytes, 10))
	}

	return nil
}

func extractFile(r io.Reader, filename string, mode fs.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}

	defer f.Close()

	src := &trackingReader{Reader: r}

	_, err = file.SafeCopy(f, src)
	if src.err != nil {
		return invalidBackupErr(src.err, "Failed to extract %q from backup", filepath.Base(filename))
	}

	if err != nil {
		return fmt.Errorf("Failed to write %q: %w", filename, err)
	}

	return f.Close()
}

// trackingReader records read errors to tell them apart from write errors.
type trackingReader struct {
	io.Reader

	err error
}

func (t *trackingReader) Read(p []byte) (int, error) {
	n, err := t.Reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		t.err = err
	}

	return n, err
}

// replacedEntryNames returns the entries replaced by the restore staged in stagedDir.
func replacedEntryNames(stagedDir string) []string {
	names := make([]string, 0, len(backupEntries)+3)
	for _, entry := range backupEntries {
		if entry.keptIfAbsent && !file.PathExists(filepath.Join(stagedDir, entry.name)) {
			continue
		}

		names = append(names, entry.name)
	}

	return append(names, databaseFilename+"-journal", databaseFilename+"-wal", databaseFilename+"-shm")
}

// ApplyRestore applies or rolls back a staged restore, it must run before the var dir is read.
func (b backup) ApplyRestore() (err error) {
	varDir := b.varDir

	err = os.RemoveAll(filepath.Join(varDir, backupDirName))
	if err != nil {
		return err
	}

	restoreDir := filepath.Join(varDir, restoreDirName)
	stagedDir := filepath.Join(restoreDir, "staged")
	previousDir := filepath.Join(restoreDir, "previous")
	markerFile := filepath.Join(restoreDir, restoreAppliedMarker)

	if file.PathExists(markerFile) {
		var marker restoreMarker
		marker, err = readRestoreMarker(markerFile)
		if err != nil {
			return err
		}

		err = rollbackRestore(varDir, previousDir, marker)
		if err != nil {
			return fmt.Errorf("Failed to roll back restore, which did not complete: %w", err)
		}

		err = os.RemoveAll(restoreDir)
		if err != nil {
			return err
		}

		return system.ErrRestoreRolledBack
	}

	if !file.PathExists(stagedDir) {
		// Clean up leftovers of an interrupted upload.
		return os.RemoveAll(restoreDir)
	}

	err = os.MkdirAll(previousDir, 0o700)
	if err != nil {
		return err
	}

	names := replacedEntryNames(stagedDir)

	// Move the current state out of the way, this is safe to repeat.
	for _, name := range names {
		if !file.PathExists(filepath.Join(varDir, name)) {
			continue
		}

		err = os.Rename(filepath.Join(varDir, name), filepath.Join(previousDir, name))
		if err != nil {
			return err
		}
	}

	marker := restoreMarker{Replaced: names}
	for _, name := range names {
		if file.PathExists(filepath.Join(previousDir, name)) {
			marker.Previous = append(marker.Previous, name)
		}
	}

	// From here on, previousDir holds the replaced state, the marker records its entries.
	err = writeRestoreMarker(markerFile, marker)
	if err != nil {
		return err
	}

	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() {
		revertErr := rollbackRestore(varDir, previousDir, marker)
		if revertErr == nil {
			revertErr = os.RemoveAll(restoreDir)
		}

		if revertErr != nil {
			err = errors.Join(err, fmt.Errorf("Failed to roll back restore: %w", revertErr))
		}
	})

	for _, name := range backupEntryNames() {
		if !file.PathExists(filepath.Join(stagedDir, name)) {
			continue
		}

		err = os.Rename(filepath.Join(stagedDir, name), filepath.Join(varDir, name))
		if err != nil {
			return err
		}
	}

	err = os.RemoveAll(stagedDir)
	if err != nil {
		return err
	}

	reverter.Success()

	return nil
}

// restoreMarker records the entries of the var dir affected by an applied restore.
type restoreMarker struct {
	// Replaced holds the entries replaced by the restore.
	Replaced []string `json:"replaced"`

	// Previous holds the replaced entries, which existed before and have been moved to the previous dir.
	Previous []string `json:"previous"`
}

func readRestoreMarker(filename string) (restoreMarker, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return restoreMarker{}, err
	}

	var marker restoreMarker

	err = json.Unmarshal(content, &marker)
	if err != nil {
		return restoreMarker{}, fmt.Errorf("Failed to read restore marker %q: %w", filename, err)
	}

	return marker, nil
}

func writeRestoreMarker(filename string, marker restoreMarker) error {
	content, err := json.Marshal(marker)
	if err != nil {
		return err
	}

	return file.WriteFileAtomic(filename, content, 0o600)
}

// rollbackRestore replaces the entries in varDir with the ones in previousDir, it is safe to repeat.
func rollbackRestore(varDir string, previousDir string, marker restoreMarker) error {
	for _, name := range marker.Replaced {
		// Already put back by a roll back, which has been interrupted.
		if slices.Contains(marker.Previous, name) && !file.PathExists(filepath.Join(previousDir, name)) {
			continue
		}

		err := os.RemoveAll(filepath.Join(varDir, name))
		if err != nil {
			return err
		}

		if !file.PathExists(filepath.Join(previousDir, name)) {
			continue
		}

		err = os.Rename(filepath.Join(previousDir, name), filepath.Join(varDir, name))
		if err != nil {
			return err
		}
	}

	return nil
}

// IsRestoreApplied reports, if a restore is in place but not yet completed.
func (b backup) IsRestoreApplied() bool {
	return file.PathExists(filepath.Join(b.varDir, restoreDirName, restoreAppliedMarker))
}

// CompleteRestore removes the state replaced by a successful restore.
func (b backup) CompleteRestore() error {
	restoreDir := filepath.Join(b.varDir, restoreDirName)

	// Removing the marker first makes the restore final, even if the cleanup is interrupted.
	err := os.Remove(filepath.Join(restoreDir, restoreAppliedMarker))
	if err != nil {
		return err
	}

	err = os.RemoveAll(filepath.Join(restoreDir, "previous"))
	if err != nil {
		return err
	}

	// The directory is kept, if a new upload has started in the meantime.
	err = os.Remove(restoreDir)
	if err != nil && !errors.Is(err, syscall.ENOTEMPTY) {
		return err
	}

	return nil
}
