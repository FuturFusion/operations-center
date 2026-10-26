package memberfiles

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

// WriteArchive writes the regular files of the directory name in store to w as a tar archive.
func (l Local) WriteArchive(w io.Writer, store string, name string) error {
	dir, err := l.path(store, name)
	if err != nil {
		return err
	}

	tw := tar.NewWriter(w)

	err = filepath.WalkDir(dir, func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(dir, filename)
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		err = tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     filepath.ToSlash(rel),
			Size:     info.Size(),
			Mode:     0o600,
			ModTime:  info.ModTime(),
		})
		if err != nil {
			return err
		}

		f, err := os.Open(filename)
		if err != nil {
			return err
		}

		defer f.Close()

		_, err = file.SafeCopy(tw, f)
		return err
	})
	if err != nil {
		return err
	}

	return tw.Close()
}
