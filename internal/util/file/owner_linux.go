//go:build linux

package file

import (
	"fmt"
	"os"
	"syscall"
)

// copyOwner applies the owner of fi with chown.
func copyOwner(fi os.FileInfo, chown func(uid int, gid int) error) error {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("Failed to get owner of %q: unexpected file info type %T", fi.Name(), fi.Sys())
	}

	return chown(int(stat.Uid), int(stat.Gid))
}
