package memberfiles

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// Local gives the other cluster members access to the files stored on this member.
type Local struct {
	varDir string
}

// NewLocal returns the stores of this member in varDir.
func NewLocal(varDir string) Local {
	return Local{varDir: varDir}
}

// Stat returns the file info of the file or directory name in store.
func (l Local) Stat(store string, name string) (os.FileInfo, error) {
	filename, err := l.path(store, name)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(filename)
	if err != nil {
		return nil, fileNotFoundErr(store, name, err)
	}

	return fi, nil
}

// Open opens the regular file name in store.
func (l Local) Open(store string, name string) (*os.File, error) {
	filename, err := l.path(store, name)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, fileNotFoundErr(store, name, err)
	}

	return f, nil
}

// Delete deletes the file or directory name in store. An empty name deletes all files of the store.
func (l Local) Delete(store string, name string) error {
	if name == "" {
		return l.deleteAll(store)
	}

	filename, err := l.path(store, name)
	if err != nil {
		return err
	}

	return os.RemoveAll(filename)
}

// deleteAll deletes all files of store, keeping its directory.
func (l Local) deleteAll(store string) error {
	dir, err := l.storeDir(store)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	var errs []error
	for _, entry := range entries {
		errs = append(errs, os.RemoveAll(filepath.Join(dir, entry.Name())))
	}

	return errors.Join(errs...)
}

func (l Local) storeDir(store string) (string, error) {
	if !slices.Contains([]string{StoreUpdates, StoreImages, StoreArtifacts}, store) {
		return "", domain.NewErrorf(domain.ErrNotFound, "", "Unknown file store %q", store)
	}

	return filepath.Join(l.varDir, store), nil
}

func (l Local) path(store string, name string) (string, error) {
	dir, err := l.storeDir(store)
	if err != nil {
		return "", err
	}

	name = filepath.FromSlash(name)
	if name == "" || !filepath.IsLocal(name) {
		return "", domain.NewValidationErrf("Invalid file name %q", name)
	}

	return filepath.Join(dir, name), nil
}

func fileNotFoundErr(store string, name string, cause error) error {
	return domain.NewErrorf(domain.ErrNotFound, "", "File %q not found in store %q", name, store).
		WithCause(cause)
}
