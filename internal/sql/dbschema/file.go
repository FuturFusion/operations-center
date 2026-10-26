package dbschema

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/lxc/incus/v7/shared/util"
)

// GetOwnerMode returns the file mode, owner UID, and owner GID for the given file.
func GetOwnerMode(fInfo os.FileInfo) (os.FileMode, int, int) {
	mode := fInfo.Mode()
	uid := int(fInfo.Sys().(*syscall.Stat_t).Uid)
	gid := int(fInfo.Sys().(*syscall.Stat_t).Gid)
	return mode, uid, gid
}

// DirCopy copies a directory recursively, overwriting the target if it exists.
func DirCopy(source string, dest string) error {
	// Get info about source.
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("failed to get source directory info: %w", err)
	}

	if !info.IsDir() {
		return errors.New("source is not a directory")
	}

	// Remove dest if it already exists.
	if util.PathExists(dest) {
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
			err := DirCopy(sourcePath, destPath)
			if err != nil {
				return fmt.Errorf("failed to copy sub-directory from %s to %s: %w", sourcePath, destPath, err)
			}
		} else {
			err := FileCopy(sourcePath, destPath)
			if err != nil {
				return fmt.Errorf("failed to copy file from %s to %s: %w", sourcePath, destPath, err)
			}
		}
	}

	return nil
}

// FileCopy copies a file, overwriting the target if it exists.
func FileCopy(source string, dest string) error {
	fi, err := os.Lstat(source)
	if err != nil {
		return err
	}

	_, uid, gid := GetOwnerMode(fi)

	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}

		if util.PathExists(dest) {
			err = os.Remove(dest)
			if err != nil {
				return err
			}
		}

		err = os.Symlink(target, dest)
		if err != nil {
			return err
		}

		if runtime.GOOS != "windows" {
			return os.Lchown(dest, uid, gid)
		}

		return nil
	}

	s, err := os.Open(source)
	if err != nil {
		return err
	}

	// The source is only read, an error on close does not matter.
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

	_, err = util.SafeCopy(d, s)
	if err != nil {
		return err
	}

	/* chown not supported on windows */
	if runtime.GOOS != "windows" {
		err = d.Chown(uid, gid)
		if err != nil {
			return err
		}
	}

	return d.Close()
}
