package memberfiles

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// DeleteStores removes all files of all stores in varDir, e.g. when the database is replaced.
func DeleteStores(varDir string) error {
	var errs []error
	for _, store := range []string{StoreUpdates, StoreImages, StoreArtifacts} {
		entries, err := os.ReadDir(filepath.Join(varDir, store))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
			continue
		}

		for _, entry := range entries {
			errs = append(errs, os.RemoveAll(filepath.Join(varDir, store, entry.Name())))
		}
	}

	return errors.Join(errs...)
}

// unknownFileMinAge is the minimum age of an unknown file before it is removed.
const unknownFileMinAge = time.Minute

// DeleteUnknownFiles deletes the files below dir whose path is not listed in known, and any directories left empty.
func DeleteUnknownFiles(dir string, known map[string]bool) error {
	var dirs []string

	err := filepath.WalkDir(dir, func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}

			return err
		}

		if filename == dir {
			return nil
		}

		if entry.IsDir() {
			dirs = append(dirs, filename)
			return nil
		}

		name, err := filepath.Rel(dir, filename)
		if err != nil {
			return err
		}

		if known[filepath.ToSlash(name)] {
			return nil
		}

		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < unknownFileMinAge {
			return nil
		}

		return os.Remove(filename)
	})
	if err != nil {
		return err
	}

	// Directories are walked in lexical order, so children come after their parents.
	for _, d := range slices.Backward(dirs) {
		err = os.Remove(d)
		if err != nil && !errors.Is(err, syscall.ENOTEMPTY) {
			return err
		}
	}

	return nil
}
