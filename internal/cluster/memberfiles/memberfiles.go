package memberfiles

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"syscall"
	"time"
)

// Stores, each one is the directory of the same name in the var dir.
const (
	StoreUpdates   = "updates"
	StoreImages    = "images"
	StoreArtifacts = "artifacts"
)

// DeleteStores removes all files of all stores in varRoot, e.g. when the database is replaced.
func DeleteStores(varRoot *os.Root) error {
	stores := []string{StoreUpdates, StoreImages, StoreArtifacts}

	errs := make([]error, 0, len(stores))
	for _, store := range stores {
		errs = append(errs, deleteStore(varRoot, store))
	}

	return errors.Join(errs...)
}

// unknownFileMinAge is the minimum age of an unknown file before it is removed.
const unknownFileMinAge = time.Minute

// DeleteUnknownFiles deletes the files in root whose path is not listed in known, and any directories left empty.
func DeleteUnknownFiles(root *os.Root, known map[string]bool) error {
	var dirs []string
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}

			return err
		}

		if name == "." {
			return nil
		}

		if entry.IsDir() {
			dirs = append(dirs, name)
			return nil
		}

		if known[name] {
			return nil
		}

		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < unknownFileMinAge {
			return nil
		}

		return root.Remove(name)
	})
	if err != nil {
		return err
	}

	// Directories are walked in lexical order, so children come after their parents.
	for _, d := range slices.Backward(dirs) {
		err = root.Remove(d)
		if err != nil && !errors.Is(err, syscall.ENOTEMPTY) {
			return err
		}
	}

	return nil
}

// DeleteUnknownFilesIn deletes the unknown files of store in varRoot like DeleteUnknownFiles.
func DeleteUnknownFilesIn(varRoot *os.Root, store string, known map[string]bool) error {
	root, err := varRoot.OpenRoot(store)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	defer root.Close()

	return DeleteUnknownFiles(root, known)
}
