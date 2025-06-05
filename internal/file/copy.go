//go:build linux
// +build linux

package file

import (
	"io"
	"os"
	"syscall"
)

// Copy copies a file, overwriting the target if it exists.
func Copy(source string, dest string) error {
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

		return os.Lchown(dest, uid, gid)
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

	_, err = io.Copy(d, s)
	if err != nil {
		return err
	}

	err = d.Chown(uid, gid)
	if err != nil {
		return err
	}

	return d.Close()
}

func GetOwnerMode(fInfo os.FileInfo) (os.FileMode, int, int) {
	mode := fInfo.Mode()
	uid := int(fInfo.Sys().(*syscall.Stat_t).Uid)
	gid := int(fInfo.Sys().(*syscall.Stat_t).Gid)
	return mode, uid, gid
}
