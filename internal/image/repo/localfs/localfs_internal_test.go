package localfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func TestLocalfs_Exists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)
		img         *image.IncusImage

		assertErr  require.ErrorAssertionFunc
		wantExists bool
	}{
		{
			name: "file exists",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				imagePath := filepath.Join("os", "release", "arch", "variant")

				err := os.MkdirAll(filepath.Join(destDir, imagePath, "1"), 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, imagePath, "1", "file1.txt"), []byte(`file1 body`), 0o600)
				require.NoError(t, err)
			},
			img: &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			},

			assertErr:  require.NoError,
			wantExists: true,
		},
		{
			name: "file does not exist",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
			},
			img: &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			},

			assertErr:  require.NoError,
			wantExists: false,
		},
		{
			name: "error - path component is not a directory",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				err := os.WriteFile(filepath.Join(destDir, "os"), []byte(`not a directory`), 0o600)
				require.NoError(t, err)
			},
			img: &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			},

			assertErr:  require.Error,
			wantExists: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs := New(fsroot.Open(t, tmpDir))

			// Run test
			exists, err := lfs.Exists(t.Context(), tc.img, "1", "file1.txt")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantExists, exists)
		})
	}
}

func TestLocalfs_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)
		img         *image.IncusImage
		filename    string

		assertErr    require.ErrorAssertionFunc
		assertReader func(t *testing.T, r io.ReadCloser)
	}{
		{
			name: "success",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				imagePath := filepath.Join("os", "release", "arch", "variant")

				err := os.MkdirAll(filepath.Join(destDir, imagePath, "1"), 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, imagePath, "1", "file1.txt"), []byte(`file1 body`), 0o600)
				require.NoError(t, err)
			},
			img: &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			},
			filename: `file1.txt`,

			assertErr: require.NoError,
			assertReader: func(t *testing.T, r io.ReadCloser) {
				t.Helper()
				gotBody, err := io.ReadAll(r)
				require.NoError(t, err)
				require.Equal(t, []byte(`file1 body`), gotBody)
			},
		},
		{
			name: "error - file not found",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				imagePath := filepath.Join("os", "release", "arch", "variant")

				err := os.MkdirAll(filepath.Join(destDir, imagePath, "1"), 0o700)
				require.NoError(t, err)
			},
			img: &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			},
			filename: `file1.txt`,

			assertErr: require.Error,
			assertReader: func(t *testing.T, r io.ReadCloser) {
				t.Helper()
				require.Nil(t, r)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs := New(fsroot.Open(t, tmpDir))

			// Run test
			gotReader, _, err := lfs.Get(t.Context(), tc.img, "1", tc.filename)

			// Assert
			tc.assertErr(t, err)
			tc.assertReader(t, gotReader)
		})
	}
}

func TestLocalfs_Put(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stream io.ReadCloser
		commit bool
		cancel bool

		assertErr       require.ErrorAssertionFunc
		assertCommitErr require.ErrorAssertionFunc
		assertCancelErr require.ErrorAssertionFunc
		wantSize        int64
	}{
		{
			name:   "success - commit",
			stream: io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			commit: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: require.NoError,
			wantSize:        6,
		},
		{
			name:   "success - commit + cancel",
			stream: io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			commit: true,
			cancel: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: require.NoError,
			wantSize:        6,
		},
		{
			name:   "cancel",
			stream: io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			cancel: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: require.NoError,
			wantSize:        6,
		},
		{
			name:   "error - stream error",
			stream: io.NopCloser(iotest.ErrReader(boom.Error)),

			assertErr:       boom.ErrorIs,
			assertCommitErr: require.NoError,
			assertCancelErr: require.NoError,
		},
		{
			name:   "error - stream close error in commit",
			stream: errCloser(bytes.NewBuffer([]byte("foobar")), boom.Error),
			commit: true,

			assertErr:       require.NoError,
			assertCommitErr: boom.ErrorIs,
			assertCancelErr: require.NoError,
			wantSize:        6,
		},
		{
			name:   "error - stream close error in cancel",
			stream: errCloser(bytes.NewBuffer([]byte("foobar")), boom.Error),
			cancel: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: boom.ErrorIs,
			wantSize:        6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			lfs := New(fsroot.Open(t, tmpDir))
			img := &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			}

			// Run test
			commit, cancel, size, err := lfs.Put(t.Context(), img, "1", "file.name", tc.stream)

			var commitErr error
			if tc.commit {
				commitErr = commit()
			}

			var cancelErr error
			if tc.cancel {
				cancelErr = cancel()
			}

			// Assert
			tc.assertErr(t, err)
			tc.assertCommitErr(t, commitErr)
			tc.assertCancelErr(t, cancelErr)
			require.Equal(t, tc.wantSize, size)
		})
	}
}

func errCloser(r io.Reader, err error) io.ReadCloser {
	return nopCloser{
		Reader: r,
		err:    err,
	}
}

type nopCloser struct {
	io.Reader

	err error
}

func (n nopCloser) Close() error { return n.err }

func TestLocalfs_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)
		img         *image.IncusImage

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				imagePath := filepath.Join("os", "release", "arch", "variant")

				err := os.MkdirAll(filepath.Join(destDir, imagePath), 0o700)
				require.NoError(t, err)
			},
			img: &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			},

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs := New(fsroot.Open(t, tmpDir))

			// Run test
			err := lfs.Delete(t.Context(), tc.img)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestLocalfs_DeleteVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)
		img         *image.IncusImage

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				imagePath := filepath.Join("os", "release", "arch", "variant")

				err := os.MkdirAll(filepath.Join(destDir, imagePath, "1"), 0o700)
				require.NoError(t, err)
			},
			img: &image.IncusImage{
				Name:            "os:release:arch:variant",
				OperatingSystem: "os",
				Release:         "release",
				Architecture:    "arch",
				Variant:         "variant",
			},

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs := New(fsroot.Open(t, tmpDir))

			// Run test
			err := lfs.DeleteVersion(t.Context(), tc.img, "1")

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestLocalfs_DeleteVersionFile(t *testing.T) {
	t.Parallel()

	// Setup
	tmpDir := t.TempDir()
	versionDir := filepath.Join(tmpDir, "os", "release", "arch", "variant", "1")
	err := os.MkdirAll(versionDir, 0o700)
	require.NoError(t, err)
	for _, filename := range []string{"file1.txt", "file2.txt"} {
		err = os.WriteFile(filepath.Join(versionDir, filename), nil, 0o600)
		require.NoError(t, err)
	}

	lfs := New(fsroot.Open(t, tmpDir))
	img := &image.IncusImage{
		Name:            "os:release:arch:variant",
		OperatingSystem: "os",
		Release:         "release",
		Architecture:    "arch",
		Variant:         "variant",
	}

	// Run test
	err = lfs.DeleteVersionFile(t.Context(), img, "1", "file1.txt")

	// Assert
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(versionDir, "file1.txt"))
	require.FileExists(t, filepath.Join(versionDir, "file2.txt"), "other files of the version are kept")
}

func TestLocalfs_invalidPath(t *testing.T) {
	t.Parallel()

	validImage := func() *image.IncusImage {
		return &image.IncusImage{
			Name:            "os:release:arch:variant",
			OperatingSystem: "os",
			Release:         "release",
			Architecture:    "arch",
			Variant:         "variant",
		}
	}

	tests := []struct {
		name string
		run  func(ctx context.Context, lfs *localfs) error
	}{
		{
			name: "Exists - version with path separator",
			run: func(ctx context.Context, lfs *localfs) error {
				_, err := lfs.Exists(ctx, validImage(), "../other/1", "file1.txt")
				return err
			},
		},
		{
			name: "Get - release is parent directory",
			run: func(ctx context.Context, lfs *localfs) error {
				img := validImage()
				img.Release = ".."
				_, _, err := lfs.Get(ctx, img, "1", "file1.txt")
				return err
			},
		},
		{
			name: "Get - file name with path separator",
			run: func(ctx context.Context, lfs *localfs) error {
				_, _, err := lfs.Get(ctx, validImage(), "1", "../../other/1/file1.txt")
				return err
			},
		},
		{
			name: "Put - variant outside of storage",
			run: func(ctx context.Context, lfs *localfs) error {
				img := validImage()
				img.Variant = "../../../../escape"
				_, cancel, _, err := lfs.Put(ctx, img, "1", "file1.txt", io.NopCloser(bytes.NewBufferString("escape")))
				return errors.Join(err, cancel())
			},
		},
		{
			name: "Delete - operating system is parent directory",
			run: func(ctx context.Context, lfs *localfs) error {
				img := validImage()
				img.OperatingSystem = ".."
				return lfs.Delete(ctx, img)
			},
		},
		{
			name: "DeleteVersion - version is parent directory",
			run: func(ctx context.Context, lfs *localfs) error {
				return lfs.DeleteVersion(ctx, validImage(), "..")
			},
		},
		{
			name: "DeleteVersionFile - file name is parent directory",
			run: func(ctx context.Context, lfs *localfs) error {
				return lfs.DeleteVersionFile(ctx, validImage(), "1", "..")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			parentDir := t.TempDir()
			storageDir := filepath.Join(parentDir, "storage")
			otherFile := filepath.Join(storageDir, "os", "release", "arch", "other", "1", "file1.txt")
			err := os.MkdirAll(filepath.Dir(otherFile), 0o700)
			require.NoError(t, err)
			err = os.WriteFile(otherFile, []byte(`file1 body`), 0o600)
			require.NoError(t, err)

			lfs := New(fsroot.Open(t, storageDir))

			// Run test
			err = tc.run(t.Context(), lfs)

			// Assert
			require.ErrorContains(t, err, "is not a valid file name")
			require.FileExists(t, otherFile, "files of other images must not be touched")

			parentEntries, err := os.ReadDir(parentDir)
			require.NoError(t, err)
			require.Len(t, parentEntries, 1, "nothing must be created outside of the storage directory")
		})
	}
}
