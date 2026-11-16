//go:build !linux

package file

import (
	"os"
)

// copyOwner does nothing. Only Linux keeps the owner on copy.
func copyOwner(_ os.FileInfo, _ func(uid int, gid int) error) error {
	return nil
}
