package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

type localfs struct {
	storageDir string
}

var _ image.ImageIncusFileRepo = localfs{}

func New(storageDir string) (*localfs, error) {
	err := os.MkdirAll(storageDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory for local update storage: %w", err)
	}

	return &localfs{
		storageDir: storageDir,
	}, nil
}

func (l localfs) Exists(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string) (bool, error) {
	fullFilename := filepath.Join(l.storageDir, img.FilePath(), versionIdentifier, filename)

	_, err := os.Stat(fullFilename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (l localfs) Get(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string) (io.ReadCloser, int64, error) {
	fullFilename := filepath.Join(l.storageDir, img.FilePath(), versionIdentifier, filename)

	fi, err := os.Stat(fullFilename)
	if err != nil {
		return nil, 0, err
	}

	f, err := os.Open(fullFilename)
	if err != nil {
		return nil, 0, err
	}

	return f, fi.Size(), nil
}

func (l localfs) Put(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string, content io.ReadCloser) (image.CommitFunc, image.CancelFunc, int64, error) {
	fullFilename := filepath.Join(l.storageDir, img.FilePath(), versionIdentifier, filename)
	temporaryFullFilename := fullFilename + ".partial"
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

		if file.PathExists(temporaryFullFilename) {
			temporaryFileRemoveErr = os.Remove(temporaryFullFilename)
		}

		return errors.Join(contentCloseErr, targetCloseErr, temporaryFileRemoveErr)
	}

	err := os.MkdirAll(filepath.Dir(fullFilename), 0o700)
	if err != nil {
		return nil, cancel, 0, err
	}

	target, err = os.OpenFile(temporaryFullFilename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		return nil, cancel, 0, err
	}

	size, err := file.SafeCopy(target, content)
	if err != nil {
		return nil, cancel, 0, err
	}

	commit := func() (err error) {
		defer func() {
			if file.PathExists(temporaryFullFilename) {
				removeErr := os.Remove(temporaryFullFilename)
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

		temporaryFileRenameErr = os.Rename(temporaryFullFilename, fullFilename)

		err = errors.Join(contentCloseErr, targetCloseErr, temporaryFileRenameErr)
		if err != nil {
			return err
		}

		committed = true

		return nil
	}

	return commit, cancel, size, err
}

func (l localfs) Delete(ctx context.Context, img *image.IncusImage) error {
	fullFilename := filepath.Join(l.storageDir, img.Path())

	return os.RemoveAll(fullFilename)
}

func (l localfs) DeleteVersion(ctx context.Context, img *image.IncusImage, versionIdentifier string) error {
	fullFilename := filepath.Join(l.storageDir, img.FilePath(), versionIdentifier)

	return os.RemoveAll(fullFilename)
}

func (l localfs) DeleteVersionFile(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string) error {
	fullFilename := filepath.Join(l.storageDir, img.FilePath(), versionIdentifier, filename)

	return os.RemoveAll(fullFilename)
}
