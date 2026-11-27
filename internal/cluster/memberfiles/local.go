package memberfiles

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// Local gives the other cluster members access to the files stored on this member.
type Local struct {
	varRoot *os.Root
}

// NewLocal returns the stores of this member in varRoot.
func NewLocal(varRoot *os.Root) Local {
	return Local{varRoot: varRoot}
}

// Stat returns the file info of the file or directory name in store.
func (l Local) Stat(store string, name string) (os.FileInfo, error) {
	filename, err := l.path(store, name)
	if err != nil {
		return nil, err
	}

	fi, err := l.varRoot.Stat(filename)
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

	f, err := l.varRoot.Open(filename)
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

	return l.varRoot.RemoveAll(filename)
}

// deleteAll deletes all files of store, keeping its directory.
func (l Local) deleteAll(store string) error {
	err := checkStore(store)
	if err != nil {
		return err
	}

	return deleteStore(l.varRoot, store)
}

// deleteStore deletes all files of the directory store in varRoot, keeping the directory.
func deleteStore(varRoot *os.Root, store string) error {
	entries, err := fs.ReadDir(varRoot.FS(), store)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	var errs []error
	for _, entry := range entries {
		errs = append(errs, varRoot.RemoveAll(path.Join(store, entry.Name())))
	}

	return errors.Join(errs...)
}

func checkStore(store string) error {
	if !slices.Contains([]string{StoreUpdates, StoreImages, StoreArtifacts}, store) {
		return domain.NewErrorf(domain.ErrNotFound, "", "Unknown file store %q", store)
	}

	return nil
}

// path returns the path of the file name in store, relative to the var dir.
func (l Local) path(store string, name string) (string, error) {
	err := checkStore(store)
	if err != nil {
		return "", err
	}

	name = filepath.FromSlash(name)
	if name == "" || !filepath.IsLocal(name) {
		return "", domain.NewValidationErrf("Invalid file name %q", name)
	}

	return filepath.Join(store, name), nil
}

func fileNotFoundErr(store string, name string, cause error) error {
	return domain.NewErrorf(domain.ErrNotFound, "", "File %q not found in store %q", name, store).
		WithCause(cause)
}
