package file

import (
	"errors"
	"io/fs"
	"os"
)

// PathExists returns true, if the given path does exist, regardless of its type
// (file, directory, symlink, etc.) and false if the given path does not exist.
func PathExists(name string) bool {
	_, err := os.Lstat(name)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	}

	return true
}
