package local

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
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/signature"
	"github.com/FuturFusion/operations-center/shared/api"
)

var UpdateSourceSpaceUUID = uuid.MustParse(`00000000-0000-0000-0000-000000000001`)

const originSuffix = " (local)"

type local struct {
	storageDir string
	verifier   signature.Verifier
}

var _ provisioning.UpdateSourceWithForgetAndAddPort = &local{}

func New(storageDir string, verifier signature.Verifier) (*local, error) {
	err := os.MkdirAll(storageDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to create storage directory: %w", err)
	}

	return &local{
		storageDir: storageDir,
		verifier:   verifier,
	}, nil
}

func (m local) GetLatest(ctx context.Context, limit int) (provisioning.Updates, error) {
	entries, err := os.ReadDir(m.storageDir)
	if err != nil {
		return nil, fmt.Errorf("Failed to read updates from %q", m.storageDir)
	}

	updates := make([]provisioning.Update, 0, len(entries))
	for _, entry := range entries {
		update, err := m.getUpdate(ctx, entry.Name())
		if err != nil {
			slog.WarnContext(ctx, "Skipping invalid update directory", logger.Err(err))
			continue
		}

		update.Origin += originSuffix
		update.UUID = uuidFromUpdate(update)

		// Fallback to x84_64 for architecture if not defined.
		for i := range update.Files {
			if update.Files[i].Architecture == api.ArchitectureUndefined {
				update.Files[i].Architecture = api.Architecture64BitIntelX86
			}
		}

		updates = append(updates, update)
	}

	sort.Slice(updates, func(i, j int) bool {
		return updates[i].PublishedAt.After(updates[j].PublishedAt)
	})

	limit = min(len(updates), limit)

	return provisioning.Updates(updates[:limit]), nil
}

func (m local) GetUpdateAllFiles(ctx context.Context, update provisioning.Update) (provisioning.UpdateFiles, error) {
	u, err := m.getUpdate(ctx, update.UUID.String())

	return u.Files, err
}

func (m local) getUpdate(_ context.Context, id string) (provisioning.Update, error) {
	updateFilename := filepath.Join(id, "update.json")
	updateFilePath := filepath.Join(m.storageDir, updateFilename)

	body, err := os.ReadFile(updateFilePath)
	if err != nil {
		return provisioning.Update{}, fmt.Errorf("Filed to read %q: %w", updateFilename, err)
	}

	update := provisioning.Update{}

	err = json.Unmarshal(body, &update)
	if err != nil {
		return provisioning.Update{}, fmt.Errorf("Failed to unmarshal %q: %w", updateFilename, err)
	}

	changelogFilename := filepath.Join(id, "changelog.txt")
	changelogFilePath := filepath.Join(m.storageDir, changelogFilename)

	body, err = os.ReadFile(changelogFilePath)
	if err != nil {
		return provisioning.Update{}, fmt.Errorf("Filed to read %q: %w", changelogFilename, err)
	}

	update.Changelog = string(body)

	// Fallback to x84_64 for architecture if not defined.
	for i := range update.Files {
		if update.Files[i].Architecture == api.ArchitectureUndefined {
			update.Files[i].Architecture = api.Architecture64BitIntelX86
		}
	}

	return update, nil
}

func (m local) GetUpdateFileByFilename(_ context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
	filename = filepath.Join(update.UUID.String(), filename)
	filePath := filepath.Join(m.storageDir, filename)

	fstat, err := os.Stat(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to open %q: %w", filename, err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to open %q: %w", filename, err)
	}

	return f, int(fstat.Size()), nil
}

func (m local) ForgetUpdate(_ context.Context, update provisioning.Update) error {
	err := os.RemoveAll(filepath.Join(m.storageDir, update.UUID.String()))
	if err != nil {
		return fmt.Errorf("Failed to forget update %q: %w", update.UUID.String(), err)
	}

	return nil
}

const tmpUpdateDirPrefix = "tmp-update-*"

func (m local) Add(ctx context.Context, tarReader *tar.Reader) (_ *provisioning.Update, err error) {
	// Ensure, storage directory is present
	err = os.MkdirAll(m.storageDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to add update: %w", err)
	}

	var tmpDir string
	tmpDir, err = os.MkdirTemp(m.storageDir, tmpUpdateDirPrefix)
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

	// Verify update.json signature.
	filename := filepath.Join(tmpDir, "update.json")
	err = m.verifier.VerifyFile(filename)
	if err != nil {
		return nil, fmt.Errorf("Failed to verify signature for %q: %w", filename, err)
	}

	delete(extractedFiles, "update.json.sig")

	// Read Changelog.
	updateManifest, err := readUpdateJSONAndChangelog(tmpDir, extractedFiles)
	if err != nil {
		return nil, err
	}

	// Skip further processing, if update with the same UUID is already present.
	_, err = os.Stat(filepath.Join(m.storageDir, updateManifest.UUID.String()))
	if err == nil {
		return updateManifest, nil
	}

	// Verify files of the update.
	err = verifyUpdateFiles(ctx, tmpDir, updateManifest, extractedFiles)
	if err != nil {
		return nil, err
	}

	// Update processed successfully, rename the temporary folder to the UUID of the update.
	err = os.Rename(tmpDir, filepath.Join(m.storageDir, updateManifest.UUID.String()))
	if err != nil {
		return nil, fmt.Errorf("Filed to rename update files folder %q to %q: %w", tmpDir, updateManifest.UUID.String(), err)
	}

	return updateManifest, nil
}

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

func readUpdateJSONAndChangelog(destDir string, extractedFiles map[string]struct{}) (*provisioning.Update, error) {
	body, err := os.ReadFile(filepath.Join(destDir, "update.json"))
	if err != nil {
		return nil, fmt.Errorf(`Invalid archive, unable to read "update.json": %w`, err)
	}

	updateManifest := &provisioning.Update{}

	err = json.Unmarshal(body, updateManifest)
	if err != nil {
		return nil, fmt.Errorf(`Invalid archive, failed to read "update.json": %w`, err)
	}

	updateManifest.Origin += originSuffix
	updateManifest.UUID = uuidFromUpdate(*updateManifest)

	delete(extractedFiles, "update.json")

	body, err = os.ReadFile(filepath.Join(destDir, "changelog.txt"))
	if err != nil {
		return nil, fmt.Errorf(`Invalid archive, unable to read "changelog.txt": %w`, err)
	}

	updateManifest.Changelog = string(body)
	delete(extractedFiles, "changelog.txt")

	return updateManifest, nil
}

func verifyUpdateFiles(ctx context.Context, destDir string, updateManifest *provisioning.Update, extractedFiles map[string]struct{}) error {
	var err error

	for _, file := range updateManifest.Files {
		err = func() error {
			f, err := os.Open(filepath.Join(destDir, file.Filename))
			if err != nil {
				return fmt.Errorf("Invalid archive, failed to open file %q mentioned in manifest: %w", file.Filename, err)
			}

			defer func() {
				err = f.Close()
				if err != nil {
					slog.WarnContext(ctx, "Failed to close file extracted from archive", slog.String("filename", file.Filename), logger.Err(err))
				}
			}()

			h := sha256.New()
			n, err := io.Copy(h, f)
			if err != nil {
				return fmt.Errorf("Failed to verify sha256 hash for file %q: %w", file.Filename, err)
			}

			if int64(file.Size) != n {
				return fmt.Errorf("Invalid archive, file size mismatch for file %q, manifest: %d, actual: %d", file.Filename, file.Size, n)
			}

			checksum := hex.EncodeToString(h.Sum(nil))
			if file.Sha256 != checksum {
				return fmt.Errorf("Invalid archive, file sha256 mismatch for file %q, manifest: %s, actual: %s", file.Filename, file.Sha256, checksum)
			}

			return nil
		}()
		if err != nil {
			return err
		}

		delete(extractedFiles, file.Filename)
	}

	if len(extractedFiles) > 0 {
		files := make([]string, 0, len(extractedFiles))
		for file := range extractedFiles {
			files = append(files, file)
		}

		return fmt.Errorf("Invalid archive, files not mentioned in the manifest found: %s", strings.Join(files, ", "))
	}

	return nil
}
