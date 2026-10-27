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

	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("Failed to open %q: %w", dir, err)
	}

	defer root.Close()

	return WriteFileAtomicIn(root, filepath.Base(filename), contents, perm)
}
