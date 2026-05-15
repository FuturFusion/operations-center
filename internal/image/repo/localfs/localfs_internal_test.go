package localfs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestLocalfs_Exists(t *testing.T) {
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
			name: "no permission",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(destDir, "os"), 0o000)
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
			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs, err := New(tmpDir)
			require.NoError(t, err)

			// Run test
			exists, err := lfs.Exists(t.Context(), tc.img, "1", "file1.txt")

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantExists, exists)
		})
	}
}

func TestLocalfs_Get(t *testing.T) {
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
			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs, err := New(tmpDir)
			require.NoError(t, err)

			// Run test
			gotReader, _, err := lfs.Get(t.Context(), tc.img, "1", tc.filename)

			// Assert
			tc.assertErr(t, err)
			tc.assertReader(t, gotReader)
		})
	}
}

func TestLocalfs_Put(t *testing.T) {
	tests := []struct {
		name   string
		img    image.IncusImage
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
			// Setup
			tmpDir := t.TempDir()
			lfs, err := New(tmpDir)
			require.NoError(t, err)

			// Run test
			commit, cancel, size, err := lfs.Put(t.Context(), &tc.img, "1", "file.name", tc.stream)

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
			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs, err := New(tmpDir)
			require.NoError(t, err)

			// Run test
			err = lfs.Delete(t.Context(), tc.img)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestLocalfs_DeleteVersion(t *testing.T) {
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
			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs, err := New(tmpDir)
			require.NoError(t, err)

			// Run test
			err = lfs.DeleteVersion(t.Context(), tc.img, "1")

			// Assert
			tc.assertErr(t, err)
		})
	}
}
