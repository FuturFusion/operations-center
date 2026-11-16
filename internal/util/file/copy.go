package file

import (
	"os"
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
