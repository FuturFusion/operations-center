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
	configUpdateMu *sync.Mutex

	storageDir string
	verifier   signature.Verifier
}

var _ provisioning.UpdateFilesRepo = localfs{}

func New(storageDir string, signatureVerificationRootCA string) (*localfs, error) {
	err := os.MkdirAll(storageDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory for local update storage: %w", err)
	}

	return &localfs{
		configUpdateMu: &sync.Mutex{},
		storageDir:     storageDir,
		verifier:       signature.NewVerifier([]byte(signatureVerificationRootCA)),
	}, nil
}

func (l localfs) Get(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
	fullFilename := filepath.Join(l.storageDir, update.UUID.String(), filename)

	fi, err := os.Stat(fullFilename)
	if err != nil {
		return nil, 0, err
	}

	f, err := os.Open(fullFilename)
	if err != nil {
		return nil, 0, err
	}

	return f, int(fi.Size()), nil
}

func (l localfs) Put(ctx context.Context, update provisioning.Update, filename string, content io.ReadCloser) (provisioning.CommitFunc, provisioning.CancelFunc, error) {
	fullFilename := filepath.Join(l.storageDir, update.UUID.String(), filename)
	temporaryFullFilename := fullFilename + ".partial"

	err := os.MkdirAll(filepath.Dir(fullFilename), 0o700)
	if err != nil {
		return nil, nil, err
	}

	target, err := os.OpenFile(temporaryFullFilename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		return nil, nil, err
	}

	_, err = io.Copy(target, content)
	if err != nil {
		return nil, nil, err
	}

	committed := false

	commit := func() (err error) {
		defer func() {
			if file.PathExists(temporaryFullFilename) {
				removeErr := os.Remove(temporaryFullFilename)
				if removeErr != nil {
					err = errors.Join(err, removeErr)
				}
			}
		}()

		err = content.Close()
		if err != nil {
			return err
		}

		err = os.Rename(temporaryFullFilename, fullFilename)
		if err != nil {
			return err
		}

		committed = true

		return nil
	}

	cancel := func() error {
		if committed {
			return nil
		}

		err := content.Close()
		if err != nil {
			return err
		}

		err = os.Remove(temporaryFullFilename)
		if err != nil {
			return err
		}

		return nil
	}

	return commit, cancel, err
}

func (l localfs) Delete(ctx context.Context, update provisioning.Update) error {
	fullFilename := filepath.Join(l.storageDir, update.UUID.String())

	return os.RemoveAll(fullFilename)
}

func (l localfs) CleanupAll(ctx context.Context) error {
	dir, err := os.ReadDir(l.storageDir)
	if err != nil {
		return fmt.Errorf("Failed to read storage directory %q: %w", l.storageDir, err)
	}

	var errs []error
	for _, entry := range dir {
		err = os.RemoveAll(filepath.Join(l.storageDir, entry.Name()))
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("Cleanup storage directory %q caused errors, operation might be still partly successful: %v", l.storageDir, errors.Join(errs...))
	}

	return nil
}

const tmpUpdateDirPrefix = "tmp-update-*"

func (l localfs) CreateFromArchive(ctx context.Context, tarReader *tar.Reader) (_ *provisioning.Update, err error) {
	// Ensure, storage directory is present
	err = os.MkdirAll(l.storageDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to add update: %w", err)
	}

	var tmpDir string
	tmpDir, err = os.MkdirTemp(l.storageDir, tmpUpdateDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("Failed to add update: %w", err)
	}

	defer func() {
		if err == nil {
			return
		}

		removeErr := os.RemoveAll(tmpDir)
		if removeErr != nil {
			slog.ErrorContext(ctx, "Failed to cleanup after unsuccessfully adding update files", slog.String("directory", tmpDir), logger.Err(removeErr))
		}
	}()

	// Extract content from tar archive.
	extractedFiles, err := extractTar(ctx, tarReader, tmpDir)
	if err != nil {
		return nil, err
	}

	// Verify update.sjson signature.
	filename := filepath.Join(tmpDir, "update.sjson")
	verifiedUpdateJSONBody, err := l.verifier.VerifyFile(filename)
	if err != nil {
		return nil, fmt.Errorf("Failed to verify signature for %q: %w", filename, err)
	}

	// Read Changelog.
	updateManifest, err := readUpdateJSONAndChangelog(verifiedUpdateJSONBody, extractedFiles)
	if err != nil {
		return nil, err
	}

	// Return an error, if update with the same UUID is already present.
	_, err = os.Stat(filepath.Join(l.storageDir, updateManifest.UUID.String()))
	if err == nil {
		return nil, fmt.Errorf("Update already existing")
	}

	// Verify files of the update.
	err = verifyUpdateFiles(ctx, tmpDir, updateManifest, extractedFiles)
	if err != nil {
		return nil, err
	}

	// Update processed successfully, rename the temporary folder to the UUID of the update.
	err = os.Rename(tmpDir, filepath.Join(l.storageDir, updateManifest.UUID.String()))
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

func extractTar(ctx context.Context, tarReader *tar.Reader, destDir string) (extractedFiles map[string]struct{}, err error) {
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
			sourceFile := filepath.Clean(hdr.Name)
			targetFile := filepath.Join(destDir, sourceFile)

			slog.DebugContext(ctx, "extract tar", slog.String("target_file", targetFile), slog.String("source_file", sourceFile))

			if !slices.Contains([]byte{tar.TypeReg, tar.TypeDir}, hdr.Typeflag) {
				return fmt.Errorf("Unsupported type for file %q", targetFile)
			}

			if hdr.Typeflag == tar.TypeDir {
				err = os.MkdirAll(targetFile, 0o755)
				if err != nil {
					return fmt.Errorf("Failed to create target directory %q: %w", targetFile, err)
				}

				return nil
			}

			f, err := os.Create(targetFile)
			if err != nil {
				return fmt.Errorf("Failed to create target file %q: %w", targetFile, err)
			}

			defer f.Close()

			n, err := io.Copy(f, tarReader)
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

func verifyUpdateFiles(ctx context.Context, destDir string, updateManifest *provisioning.Update, extractedFiles map[string]struct{}) error {
	var err error

	for _, updateFile := range updateManifest.Files {
		err = func() error {
			f, err := os.Open(filepath.Join(destDir, updateFile.Filename))
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
			n, err := io.Copy(h, f)
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
		err = os.Remove(filepath.Join(destDir, entry))
		if err != nil {
			return fmt.Errorf("Failed to delete extra file %q: %w", entry, err)
		}
	}

	return nil
}

func (l *localfs) UpdateConfig(_ context.Context, signatureVerificationRootCA string) {
	l.configUpdateMu.Lock()
	defer l.configUpdateMu.Unlock()

	l.verifier = signature.NewVerifier([]byte(signatureVerificationRootCA))
}
