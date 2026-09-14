package file

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes contents to the file named by filename, replacing it if
// it exists. The contents are written to a temporary file in the same directory
// first, which is then renamed into place, so a failure never leaves a partially
// written file behind. The rename is made durable before returning.
//
// In contrast to os.WriteFile, perm is applied as is and not masked by the
// umask, so a file which must not be world readable really is not.
func WriteFileAtomic(filename string, contents []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)

	f, err := os.CreateTemp(dir, "."+filepath.Base(filename)+".*")
	if err != nil {
		return fmt.Errorf("Failed to create temporary file in %q: %w", dir, err)
	}

	tmpName := f.Name()

	defer func() {
		f.Close()
		os.Remove(tmpName)
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

	err = os.Rename(tmpName, filename)
	if err != nil {
		return fmt.Errorf("Failed to move temporary file into place as %q: %w", filename, err)
	}

	// Sync the directory, so the rename survives a crash.
	d, err := os.Open(dir)
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
