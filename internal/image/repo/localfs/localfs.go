package localfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

type localfs struct {
	root *os.Root
}

var _ image.ImageIncusFileRepo = localfs{}

// New returns an incus image files repository, which stores the files in root.
func New(root *os.Root) *localfs {
	return &localfs{
		root: root,
	}
}

// imagePath returns the path of img, extended by elements, relative to the
// root. Every path element must be a single file name. This keeps the path
// inside the root.
func imagePath(img *image.IncusImage, elements ...string) (string, error) {
	for _, element := range append([]string{img.OperatingSystem, img.Release, img.Architecture, img.Variant}, elements...) {
		err := image.ValidateIncusImageFilename(element)
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(append([]string{img.FilePath()}, elements...)...), nil
}

func (l localfs) Exists(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string) (bool, error) {
	name, err := imagePath(img, versionIdentifier, filename)
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

func (l localfs) Get(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string) (io.ReadCloser, int64, error) {
	name, err := imagePath(img, versionIdentifier, filename)
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

	return f, fi.Size(), nil
}

func (l localfs) Put(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string, content io.ReadCloser) (image.CommitFunc, image.CancelFunc, int64, error) {
	name, err := imagePath(img, versionIdentifier, filename)
	if err != nil {
		return nil, content.Close, 0, err
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
		return nil, cancel, 0, err
	}

	target, err = l.root.OpenFile(temporaryName, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		return nil, cancel, 0, err
	}

	size, err := file.SafeCopy(target, content)
	if err != nil {
		return nil, cancel, 0, err
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

	return commit, cancel, size, err
}

func (l localfs) Delete(ctx context.Context, img *image.IncusImage) error {
	return l.removeAll(img)
}

func (l localfs) DeleteVersion(ctx context.Context, img *image.IncusImage, versionIdentifier string) error {
	return l.removeAll(img, versionIdentifier)
}

func (l localfs) DeleteVersionFile(ctx context.Context, img *image.IncusImage, versionIdentifier string, filename string) error {
	return l.removeAll(img, versionIdentifier, filename)
}

func (l localfs) removeAll(img *image.IncusImage, elements ...string) error {
	name, err := imagePath(img, elements...)
	if err != nil {
		return err
	}

	return l.root.RemoveAll(name)
}
