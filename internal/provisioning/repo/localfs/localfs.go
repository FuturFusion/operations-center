package localfs

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/signature"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

type localfs struct {
	root *os.Root

	verifier   signature.Verifier
	verifierMu *sync.Mutex
}

var _ provisioning.UpdateFilesRepo = localfs{}

// New returns an update files repository, which stores the files in root.
func New(root *os.Root, signatureVerificationRootCA string) *localfs {
	return &localfs{
		verifierMu: &sync.Mutex{},
		root:       root,
		verifier:   signature.NewVerifier([]byte(signatureVerificationRootCA)),
	}
}

// updateFilename returns the name of filename relative to the root.
// Names, which leave the directory of the update, are rejected.
func updateFilename(update provisioning.Update, filename string) (string, error) {
	if !filepath.IsLocal(filename) {
		return "", fmt.Errorf("Invalid file name %q for update %q", filename, update.UUID.String())
	}

	return filepath.Join(update.UUID.String(), filename), nil
}

func (l localfs) Exists(ctx context.Context, update provisioning.Update, filename string) (bool, error) {
	name, err := updateFilename(update, filename)
	if err != nil {
		return false, err
	}

	_, err = l.root.Stat(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (l localfs) Get(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
	name, err := updateFilename(update, filename)
	if err != nil {
		return nil, 0, err
	}

	f, err := l.root.Open(name)
	if err != nil {
		return nil, 0, err
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}

	return f, int(fi.Size()), nil
}

func (l localfs) Put(ctx context.Context, update provisioning.Update, filename string, content io.ReadCloser) (provisioning.CommitFunc, provisioning.CancelFunc, error) {
	name, err := updateFilename(update, filename)
	if err != nil {
		return nil, content.Close, err
	}

	temporaryName := name + ".partial"
	var target *os.File
	committed := false

	cancel := func() error {
		if committed {
			return nil
		}

		var contentCloseErr error
		var targetCloseErr error
		var temporaryFileRemoveErr error

		contentCloseErr = content.Close()

		if target != nil {
			targetCloseErr = target.Close()
		}

		if file.PathExistsIn(l.root, temporaryName) {
			temporaryFileRemoveErr = l.root.Remove(temporaryName)
		}

		return errors.Join(contentCloseErr, targetCloseErr, temporaryFileRemoveErr)
	}

	err = l.root.MkdirAll(filepath.Dir(name), 0o700)
	if err != nil {
		return nil, cancel, err
	}

	target, err = l.root.OpenFile(temporaryName, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		return nil, cancel, err
	}

	_, err = file.SafeCopy(target, content)
	if err != nil {
		return nil, cancel, err
	}

	commit := func() (err error) {
		defer func() {
			if file.PathExistsIn(l.root, temporaryName) {
				removeErr := l.root.Remove(temporaryName)
				if removeErr != nil {
					err = errors.Join(err, removeErr)
				}
			}
		}()

		var contentCloseErr error
		var targetCloseErr error
		var temporaryFileRenameErr error

		contentCloseErr = content.Close()

		if target != nil {
			targetCloseErr = target.Close()
		}

		temporaryFileRenameErr = l.root.Rename(temporaryName, name)

		err = errors.Join(contentCloseErr, targetCloseErr, temporaryFileRenameErr)
		if err != nil {
			return err
		}

		committed = true

		return nil
	}

	return commit, cancel, err
}

func (l localfs) Delete(ctx context.Context, update provisioning.Update) error {
	return l.root.RemoveAll(update.UUID.String())
}

func (l localfs) PruneFiles(ctx context.Context, update provisioning.Update) error {
	// Remove all files from the update, that are not required by the update.
	basePath := update.UUID.String()

	filesLookup := make(map[string]bool, len(update.Files))
	for _, updateFile := range update.Files {
		filesLookup[updateFile.Filename] = true
	}

	err := fs.WalkDir(l.root.FS(), basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			// Skip the basePath directory.
			return nil
		}

		if d.IsDir() {
			// Ignore directories. If all files are removed, an empty directory might
			// be kept, but this is ok and will get cleaned up when the update is
			// obsolete.
			return nil
		}

		if !filesLookup[relPath] {
			err = l.root.RemoveAll(path)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

func (l localfs) CleanupAll(ctx context.Context) error {
	dir, err := fs.ReadDir(l.root.FS(), ".")
	if err != nil {
		return fmt.Errorf("Failed to read storage directory %q: %w", l.root.Name(), err)
	}

	var errs []error
	for _, entry := range dir {
		err = l.root.RemoveAll(entry.Name())
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("Cleanup storage directory %q caused errors, operation might be still partly successful: %v", l.root.Name(), errors.Join(errs...))
	}

	return nil
}

// DeleteUnknown removes the files of all updates, which are not in known.
func (l localfs) DeleteUnknown(ctx context.Context, known []uuid.UUID) error {
	dir, err := fs.ReadDir(l.root.FS(), ".")
	if err != nil {
		return fmt.Errorf("Failed to read storage directory %q: %w", l.root.Name(), err)
	}

	var errs []error
	for _, entry := range dir {
		id, err := uuid.Parse(entry.Name())
		if err == nil && slices.Contains(known, id) {
			continue
		}

		err = l.root.RemoveAll(entry.Name())
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

const tmpUpdateDirPrefix = "tmp-update-*"

func (l localfs) CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (_ *provisioning.Update, err error) {
	var tmpDir string
	tmpDir, err = file.MkdirTempIn(l.root, ".", tmpUpdateDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("Failed to add update: %w", err)
	}

	defer func() {
		if err == nil {
			return
		}

		removeErr := l.root.RemoveAll(tmpDir)
		if removeErr != nil {
			slog.ErrorContext(ctx, "Failed to cleanup after unsuccessfully adding update files", slog.String("directory", tmpDir), logger.Err(removeErr))
		}
	}()

	tmpRoot, err := l.root.OpenRoot(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("Failed to add update: %w", err)
	}

	defer tmpRoot.Close()

	// Extract content from tar archive.
	extractedFiles, err := extractTar(ctx, tarReader, tmpRoot)
	if err != nil {
		return nil, err
	}

	// Verify update.sjson signature.
	updateSJSONBody, err := tmpRoot.ReadFile("update.sjson")
	if err != nil {
		return nil, fmt.Errorf(`Invalid archive, failed to read "update.sjson": %w`, err)
	}

	l.verifierMu.Lock()
	verifiedUpdateJSONBody, err := l.verifier.Verify(updateSJSONBody)
	l.verifierMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf(`Failed to verify signature for "update.sjson": %w`, err)
	}

	// Read Changelog.
	updateManifest, err := readUpdateJSONAndChangelog(verifiedUpdateJSONBody, extractedFiles)
	if err != nil {
		return nil, err
	}

	// Return an error, if update with the same UUID is already present.
	_, err = l.root.Stat(updateManifest.UUID.String())
	if err == nil {
		return nil, fmt.Errorf("Update already existing")
	}

	// Verify files of the update.
	err = verifyUpdateFiles(ctx, tmpRoot, updateManifest, extractedFiles)
	if err != nil {
		return nil, err
	}

	// Update processed successfully, rename the temporary folder to the UUID of the update.
	err = l.root.Rename(tmpDir, updateManifest.UUID.String())
	if err != nil {
		return nil, fmt.Errorf("Filed to rename update files folder %q to %q: %w", tmpDir, updateManifest.UUID.String(), err)
	}

	return updateManifest, nil
}

type Update struct {
	Format      string                              `json:"format"`
	Channels    provisioning.UpdateUpstreamChannels `json:"channels"`
	Files       provisioning.UpdateFiles            `json:"files"`
	Origin      string                              `json:"origin"`
	PublishedAt time.Time                           `json:"published_at"`
	Severity    images.UpdateSeverity               `json:"severity"`
	Version     string                              `json:"version"`
	URL         string                              `json:"url"`
}

var UpdateSourceSpaceUUID = uuid.MustParse(`00000000-0000-0000-0000-000000000001`)

const originSuffix = " (local)"

const idSeparator = ":"

func uuidFromUpdate(u Update) uuid.UUID {
	upstreamChannels := u.Channels
	sort.Strings(upstreamChannels)

	identifier := strings.Join([]string{
		u.Origin,
		u.Version,
	}, idSeparator)

	return uuid.NewSHA1(UpdateSourceSpaceUUID, []byte(identifier))
}

func extractTar(ctx context.Context, tarReader *tar.Reader, destRoot *os.Root) (extractedFiles map[string]struct{}, err error) {
	extractedFiles = make(map[string]struct{}, 20)
	for {
		var hdr *tar.Header

		hdr, err = tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("Filed to read tar archive: %w", err)
		}

		err = func() error {
			if !filepath.IsLocal(hdr.Name) {
				return fmt.Errorf("Invalid archive, file name %q is not a relative path inside the archive", hdr.Name)
			}

			sourceFile := filepath.Clean(hdr.Name)
			targetFile := filepath.Join(destRoot.Name(), sourceFile)

			slog.DebugContext(ctx, "extract tar", slog.String("target_file", targetFile), slog.String("source_file", sourceFile))

			if !slices.Contains([]byte{tar.TypeReg, tar.TypeDir}, hdr.Typeflag) {
				return fmt.Errorf("Unsupported type for file %q", targetFile)
			}

			if hdr.Typeflag == tar.TypeDir {
				err = destRoot.MkdirAll(sourceFile, 0o755)
				if err != nil {
					return fmt.Errorf("Failed to create target directory %q: %w", targetFile, err)
				}

				return nil
			}

			f, err := destRoot.Create(sourceFile)
			if err != nil {
				return fmt.Errorf("Failed to create target file %q: %w", targetFile, err)
			}

			defer f.Close()

			n, err := file.SafeCopy(f, tarReader)
			if err != nil {
				return fmt.Errorf("Failed to write target file %q: %w", targetFile, err)
			}

			if n != hdr.Size {
				return fmt.Errorf("Size missmatch for %q, wrote %d, expected %d bytes", sourceFile, n, hdr.Size)
			}

			extractedFiles[sourceFile] = struct{}{}

			return nil
		}()
		if err != nil {
			return nil, err
		}
	}

	return extractedFiles, nil
}

func readUpdateJSONAndChangelog(updateJSONBody []byte, extractedFiles map[string]struct{}) (*provisioning.Update, error) {
	updateManifest := Update{}

	err := json.Unmarshal(updateJSONBody, &updateManifest)
	if err != nil {
		return nil, fmt.Errorf(`Invalid archive, failed to read "update.sjson": %w`, err)
	}

	updateManifest.Origin += originSuffix

	update := &provisioning.Update{
		Format:      updateManifest.Format,
		Origin:      updateManifest.Origin,
		PublishedAt: updateManifest.PublishedAt,
		Severity:    updateManifest.Severity,
		Channels:    []string{config.GetUpdates().UpdatesDefaultChannel},
		Version:     updateManifest.Version,
		URL:         updateManifest.URL,

		UpstreamChannels: updateManifest.Channels,
		Status:           api.UpdateStatusUnknown,
		UUID:             uuidFromUpdate(updateManifest),
	}

	// Process files from manifest, same logic as in updateserver.GetLatest.
	files := make(provisioning.UpdateFiles, 0, len(updateManifest.Files))
	for _, fileEntry := range updateManifest.Files {
		_, ok := images.UpdateFileComponents[fileEntry.Component]
		if !ok {
			// Remove and skip unknown file components.
			delete(extractedFiles, fileEntry.Filename)
			continue
		}

		// Handle partial uploads.
		_, ok = extractedFiles[fileEntry.Filename]
		if !ok {
			continue
		}

		files = append(files, fileEntry)
	}

	update.Files = files

	delete(extractedFiles, "update.sjson")
	delete(extractedFiles, "update.json")

	return update, nil
}

func verifyUpdateFiles(ctx context.Context, destRoot *os.Root, updateManifest *provisioning.Update, extractedFiles map[string]struct{}) error {
	var err error

	for _, updateFile := range updateManifest.Files {
		err = func() error {
			f, err := destRoot.Open(updateFile.Filename)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}

				return fmt.Errorf("Invalid archive, failed to open file %q mentioned in manifest: %w", updateFile.Filename, err)
			}

			defer func() {
				err = f.Close()
				if err != nil {
					slog.WarnContext(ctx, "Failed to close file extracted from archive", slog.String("filename", updateFile.Filename), logger.Err(err))
				}
			}()

			h := sha256.New()
			n, err := file.SafeCopy(h, f)
			if err != nil {
				return fmt.Errorf("Failed to verify sha256 hash for file %q: %w", updateFile.Filename, err)
			}

			if int64(updateFile.Size) != n {
				return fmt.Errorf("Invalid archive, file size mismatch for file %q, manifest: %d, actual: %d", updateFile.Filename, updateFile.Size, n)
			}

			checksum := hex.EncodeToString(h.Sum(nil))
			if updateFile.Sha256 != checksum {
				return fmt.Errorf("Invalid archive, file sha256 mismatch for file %q, manifest: %s, actual: %s", updateFile.Filename, updateFile.Sha256, checksum)
			}

			return nil
		}()
		if err != nil {
			return err
		}

		delete(extractedFiles, updateFile.Filename)
	}

	// Delete any extra file.
	for entry := range extractedFiles {
		err = destRoot.Remove(entry)
		if err != nil {
			return fmt.Errorf("Failed to delete extra file %q: %w", entry, err)
		}
	}

	return nil
}

func (l *localfs) UpdateConfig(_ context.Context, signatureVerificationRootCA string) {
	l.verifierMu.Lock()
	defer l.verifierMu.Unlock()

	l.verifier = signature.NewVerifier([]byte(signatureVerificationRootCA))
}
