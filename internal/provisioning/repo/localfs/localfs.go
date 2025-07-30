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
	"strings"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/signature"
	"github.com/FuturFusion/operations-center/shared/api"
)

type localfs struct {
	storageDir string
	verifier   signature.Verifier
}

var _ provisioning.UpdateFilesRepo = localfs{}

func New(storageDir string, verifier signature.Verifier) (localfs, error) {
	err := os.MkdirAll(storageDir, 0o700)
	if err != nil {
		return localfs{}, fmt.Errorf("Failed to create directory for local update storage: %w", err)
	}

	return localfs{
		storageDir: storageDir,
		verifier:   verifier,
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
	extractedFiles, err := extractTar(tarReader, tmpDir)
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
	updateManifest, err := readUpdateJSONAndChangelog(verifiedUpdateJSONBody, tmpDir, extractedFiles)
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

var UpdateSourceSpaceUUID = uuid.MustParse(`00000000-0000-0000-0000-000000000001`)

const originSuffix = " (local)"

const idSeparator = ":"

func uuidFromUpdate(u provisioning.Update) uuid.UUID {
	identifier := strings.Join([]string{
		u.Origin,
		u.Channel,
		u.Version,
		u.PublishedAt.String(),
	}, idSeparator)

	return uuid.NewSHA1(UpdateSourceSpaceUUID, []byte(identifier))
}

func extractTar(tarReader *tar.Reader, destDir string) (extractedFiles map[string]struct{}, err error) {
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
			targetFile := filepath.Join(destDir, hdr.Name)
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
				return fmt.Errorf("Size missmatch for %q, wrote %d, expected %d bytes", hdr.Name, n, hdr.Size)
			}

			return nil
		}()
		if err != nil {
			return nil, err
		}

		extractedFiles[hdr.Name] = struct{}{}
	}

	return extractedFiles, nil
}

func readUpdateJSONAndChangelog(updateJSONBody []byte, destDir string, extractedFiles map[string]struct{}) (*provisioning.Update, error) {
	updateManifest := &provisioning.Update{
		Status: api.UpdateStatusUnknown,
	}

	err := json.Unmarshal(updateJSONBody, updateManifest)
	if err != nil {
		return nil, fmt.Errorf(`Invalid archive, failed to read "update.sjson": %w`, err)
	}

	updateManifest.Origin += originSuffix
	updateManifest.UUID = uuidFromUpdate(*updateManifest)

	delete(extractedFiles, "update.sjson")
	delete(extractedFiles, "update.json")

	body, err := os.ReadFile(filepath.Join(destDir, "changelog.txt"))
	if err != nil {
		return nil, fmt.Errorf(`Invalid archive, unable to read "changelog.txt": %w`, err)
	}

	updateManifest.Changelog = string(body)
	delete(extractedFiles, "changelog.txt")

	return updateManifest, nil
}

func verifyUpdateFiles(ctx context.Context, destDir string, updateManifest *provisioning.Update, extractedFiles map[string]struct{}) error {
	var err error

	for _, updateFile := range updateManifest.Files {
		err = func() error {
			f, err := os.Open(filepath.Join(destDir, updateFile.Filename))
			if err != nil {
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

	if len(extractedFiles) > 0 {
		files := make([]string, 0, len(extractedFiles))
		for extractedFile := range extractedFiles {
			files = append(files, extractedFile)
		}

		return fmt.Errorf("Invalid archive, files not mentioned in the manifest found: %s", strings.Join(files, ", "))
	}

	return nil
}
