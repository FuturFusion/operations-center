package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Copy copies a file, overwriting the target if it exists.
func Copy(source string, dest string) error {
	fi, err := os.Lstat(source)
	if err != nil {
		return err
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}

		if PathExists(dest) {
			err = os.Remove(dest)
			if err != nil {
				return err
			}
		}

		err = os.Symlink(target, dest)
		if err != nil {
			return err
		}

		return copyOwner(fi, func(uid int, gid int) error {
			return os.Lchown(dest, uid, gid)
		})
	}

	s, err := os.Open(source)
	if err != nil {
		return err
	}

	defer func() { _ = s.Close() }()

	d, err := os.Create(dest)
	if err != nil {
		if !os.IsExist(err) {
			return err
		}

		d, err = os.OpenFile(dest, os.O_WRONLY, fi.Mode())
		if err != nil {
			return err
		}
	}

	_, err = SafeCopy(d, s)
	if err != nil {
		return err
	}

	err = copyOwner(fi, d.Chown)
	if err != nil {
		return err
	}

	return d.Close()
}

// CopyDir copies a directory recursively, overwriting the target if it exists.
func CopyDir(source string, dest string) error {
	// Get info about source.
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("failed to get source directory info: %w", err)
	}

	if !info.IsDir() {
		return errors.New("source is not a directory")
	}

	// Remove dest if it already exists.
	if PathExists(dest) {
		err := os.RemoveAll(dest)
		if err != nil {
			return fmt.Errorf("failed to remove destination directory %s: %w", dest, err)
		}
	}

	// Create dest.
	err = os.MkdirAll(dest, info.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dest, err)
	}

	// Copy all files.
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("failed to read source directory %s: %w", source, err)
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			err := CopyDir(sourcePath, destPath)
			if err != nil {
				return fmt.Errorf("failed to copy sub-directory from %s to %s: %w", sourcePath, destPath, err)
			}
		} else {
			err := Copy(sourcePath, destPath)
			if err != nil {
				return fmt.Errorf("failed to copy file from %s to %s: %w", sourcePath, destPath, err)
			}
		}
	}

	return nil
}
