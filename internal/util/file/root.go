package file

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The same limit as in os.CreateTemp and os.MkdirTemp.
const maxTempNameAttempts = 10000

// MkdirAllOpenRoot creates the directory dir and all missing parents. Then it
// opens dir as root.
func MkdirAllOpenRoot(dir string, perm os.FileMode) (*os.Root, error) {
	err := os.MkdirAll(dir, perm)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory %q: %w", dir, err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to open root %q: %w", dir, err)
	}

	return root, nil
}

// MkdirAllOpenSubRoot creates the directory name in parent and all missing
// intermediate directories. Then it opens name as root.
func MkdirAllOpenSubRoot(parent *os.Root, name string, perm os.FileMode) (*os.Root, error) {
	err := parent.MkdirAll(name, perm)
	if err != nil {
		return nil, fmt.Errorf("Failed to create directory %q in %q: %w", name, parent.Name(), err)
	}

	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("Failed to open root %q in %q: %w", name, parent.Name(), err)
	}

	return root, nil
}

// PathExistsIn works like PathExists, but resolves name in root.
func PathExistsIn(root *os.Root, name string) bool {
	_, err := root.Lstat(name)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	}

	return true
}

// CreateTempIn works like os.CreateTemp, but creates the file in the directory
// dir in root. It returns the file and its name relative to root.
func CreateTempIn(root *os.Root, dir string, pattern string) (*os.File, string, error) {
	prefix, suffix, err := splitTempPattern(pattern)
	if err != nil {
		return nil, "", err
	}

	for range maxTempNameAttempts {
		name := filepath.Join(dir, prefix+randomTempPart()+suffix)

		f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}

		if err != nil {
			return nil, "", fmt.Errorf("Failed to create temporary file %q in %q: %w", name, root.Name(), err)
		}

		return f, name, nil
	}

	return nil, "", fmt.Errorf("Failed to create temporary file in %q: %w", filepath.Join(root.Name(), dir), fs.ErrExist)
}

// MkdirTempIn works like os.MkdirTemp, but creates the directory in the
// directory dir in root. It returns the name relative to root.
func MkdirTempIn(root *os.Root, dir string, pattern string) (string, error) {
	prefix, suffix, err := splitTempPattern(pattern)
	if err != nil {
		return "", err
	}

	for range maxTempNameAttempts {
		name := filepath.Join(dir, prefix+randomTempPart()+suffix)

		err := root.Mkdir(name, 0o700)
		if errors.Is(err, fs.ErrExist) {
			continue
		}

		if err != nil {
			return "", fmt.Errorf("Failed to create temporary directory %q in %q: %w", name, root.Name(), err)
		}

		return name, nil
	}

	return "", fmt.Errorf("Failed to create temporary directory in %q: %w", filepath.Join(root.Name(), dir), fs.ErrExist)
}

// WriteFileAtomicIn works like WriteFileAtomic, but resolves name in root.
func WriteFileAtomicIn(root *os.Root, name string, contents []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(name)

	f, tmpName, err := CreateTempIn(root, dir, "."+filepath.Base(name)+".*")
	if err != nil {
		return err
	}

	renamed := false
	defer func() {
		_ = f.Close()

		if renamed {
			return
		}

		removeErr := root.Remove(tmpName)
		if removeErr != nil {
			err = errors.Join(err, fmt.Errorf("Failed to remove temporary file %q: %w", tmpName, removeErr))
		}
	}()

	err = f.Chmod(perm)
	if err != nil {
		return fmt.Errorf("Failed to set permissions on temporary file %q: %w", tmpName, err)
	}

	_, err = f.Write(contents)
	if err != nil {
		return fmt.Errorf("Failed to write temporary file %q: %w", tmpName, err)
	}

	err = f.Sync()
	if err != nil {
		return fmt.Errorf("Failed to sync temporary file %q: %w", tmpName, err)
	}

	err = f.Close()
	if err != nil {
		return fmt.Errorf("Failed to close temporary file %q: %w", tmpName, err)
	}

	err = root.Rename(tmpName, name)
	if err != nil {
		return fmt.Errorf("Failed to move temporary file into place as %q: %w", name, err)
	}

	renamed = true

	// Sync the directory, so the rename survives a crash.
	d, err := root.Open(dir)
	if err != nil {
		return fmt.Errorf("Failed to open %q: %w", dir, err)
	}

	defer d.Close()

	err = d.Sync()
	if err != nil {
		return fmt.Errorf("Failed to sync %q: %w", dir, err)
	}

	return d.Close()
}

// splitTempPattern splits pattern at the last "*", like os.CreateTemp does.
func splitTempPattern(pattern string) (prefix string, suffix string, _ error) {
	if strings.ContainsRune(pattern, os.PathSeparator) {
		return "", "", fmt.Errorf("Pattern %q contains a path separator", pattern)
	}

	pos := strings.LastIndexByte(pattern, '*')
	if pos == -1 {
		return pattern, "", nil
	}

	return pattern[:pos], pattern[pos+1:], nil
}

func randomTempPart() string {
	return strconv.FormatUint(uint64(rand.Uint32()), 10)
}
