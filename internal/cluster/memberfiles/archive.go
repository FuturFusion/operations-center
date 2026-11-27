package memberfiles

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

// WriteArchive writes the regular files of the directory name in store to w as a tar archive.
func (l Local) WriteArchive(w io.Writer, store string, name string) error {
	dir, err := l.path(store, name)
	if err != nil {
		return err
	}

	fi, err := l.varRoot.Stat(dir)
	if err != nil || !fi.IsDir() {
		return fileNotFoundErr(store, name, err)
	}

	dirFS, err := fs.Sub(l.varRoot.FS(), filepath.ToSlash(dir))
	if err != nil {
		return err
	}

	tw := tar.NewWriter(w)

	err = fs.WalkDir(dirFS, ".", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.Type().IsRegular() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		err = tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     filename,
			Size:     info.Size(),
			Mode:     0o600,
			ModTime:  info.ModTime(),
		})
		if err != nil {
			return err
		}

		f, err := dirFS.Open(filename)
		if err != nil {
			return err
		}

		defer func() { _ = f.Close() }()

		_, err = file.SafeCopy(tw, f)
		return err
	})
	if err != nil {
		return err
	}

	return tw.Close()
}

// ExtractArchive replaces the directory name in store with the regular files of the tar archive r.
func (l Local) ExtractArchive(r io.Reader, store string, name string) (err error) {
	dir, err := l.path(store, name)
	if err != nil {
		return err
	}

	err = l.varRoot.MkdirAll(filepath.Dir(dir), 0o700)
	if err != nil {
		return err
	}

	tmpDir := filepath.Join(filepath.Dir(dir), "."+filepath.Base(dir)+".partial-"+uuid.NewString())

	err = l.varRoot.Mkdir(tmpDir, 0o700)
	if err != nil {
		return err
	}

	defer func() {
		removeErr := l.varRoot.RemoveAll(tmpDir)
		if err == nil && removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			err = removeErr
		}
	}()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		rel := filepath.FromSlash(header.Name)
		if !filepath.IsLocal(rel) {
			return domain.NewValidationErrf("Invalid file name %q in archive", header.Name)
		}

		err = l.extractFile(tr, filepath.Join(tmpDir, rel))
		if err != nil {
			return err
		}
	}

	err = l.varRoot.RemoveAll(dir)
	if err != nil {
		return err
	}

	return l.varRoot.Rename(tmpDir, dir)
}

func (l Local) extractFile(r io.Reader, filename string) error {
	err := l.varRoot.MkdirAll(filepath.Dir(filename), 0o700)
	if err != nil {
		return err
	}

	f, err := l.varRoot.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	_, err = file.SafeCopy(f, r)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("Failed to write %q: %w", filename, err)
	}

	return f.Close()
}
