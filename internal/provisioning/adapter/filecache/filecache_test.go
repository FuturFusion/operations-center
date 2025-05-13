package filecache_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/filecache"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func TestFilecache_GetLatest(t *testing.T) {
	tests := []struct {
		name                   string
		sourceGetLatestUpdates provisioning.Updates
		sourceGetLatestErr     error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			sourceGetLatestUpdates: provisioning.Updates{
				{
					UUID: uuid.MustParse(`bcc98445-16a5-4676-9512-f8c31ec4b510`),
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error",

			sourceGetLatestErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &mock.UpdateSourcePortMock{
				GetLatestFunc: func(ctx context.Context, limit int) (provisioning.Updates, error) {
					return tc.sourceGetLatestUpdates, tc.sourceGetLatestErr
				},
			}

			f := filecache.New(source, "")

			updates, err := f.GetLatest(context.Background(), 3)

			tc.assertErr(t, err)
			require.Equal(t, updates, tc.sourceGetLatestUpdates)
		})
	}
}

func TestFilecache_GetUpdateAllFiles(t *testing.T) {
	tests := []struct {
		name                         string
		sourceGetAllFilesUpdateFiles provisioning.UpdateFiles
		sourceGetAllFilesErr         error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			sourceGetAllFilesUpdateFiles: provisioning.UpdateFiles{
				{
					Filename: "dummy.txt",
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error",

			sourceGetAllFilesErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &mock.UpdateSourcePortMock{
				GetUpdateAllFilesFunc: func(ctx context.Context, update provisioning.Update) (provisioning.UpdateFiles, error) {
					return tc.sourceGetAllFilesUpdateFiles, tc.sourceGetAllFilesErr
				},
			}

			f := filecache.New(source, "")

			files, err := f.GetUpdateAllFiles(context.Background(), provisioning.Update{})

			tc.assertErr(t, err)
			require.Equal(t, files, tc.sourceGetAllFilesUpdateFiles)
		})
	}
}

func TestFilecache_GetUpdateFileByFilename(t *testing.T) {
	id := uuid.MustParse(`44914983-9658-45ed-9ec4-0a016d5570a5`)
	filename := `dummy.txt`
	content := bytes.Repeat([]byte("0123456789"), 4000) // Generate 40k content in order to exceed the default buffer size of io.Copy of 32*1024 bytes

	tests := []struct {
		name                                    string
		sourceGetUpdateFileByFilenameReadCloser io.ReadCloser
		sourceGetUpdateFileByFilenameSize       int
		sourceGetUpdateFileByFilenameErr        error
		setupCache                              func(t *testing.T, cachePath string)

		assertErr require.ErrorAssertionFunc
		assert    func(t *testing.T, stream io.ReadCloser, size int, cachePath string)
	}{
		{
			name: "success - file not in cache",

			sourceGetUpdateFileByFilenameReadCloser: io.NopCloser(bytes.NewBuffer(content)),
			sourceGetUpdateFileByFilenameSize:       len(content),
			setupCache: func(t *testing.T, path string) {
				t.Helper()
			},

			assertErr: require.NoError,
			assert: func(t *testing.T, stream io.ReadCloser, size int, cachePath string) {
				t.Helper()

				defer stream.Close()
				gotContent, err := io.ReadAll(stream)
				require.NoError(t, err)

				require.Equal(t, content, gotContent)
				require.Equal(t, len(content), size)

				assertFile(t, filepath.Join(cachePath, id.String(), filename), len(content))
			},
		},
		{
			name: "success - file in cache",

			setupCache: func(t *testing.T, cachePath string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(cachePath, id.String()), 0o700)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(cachePath, id.String(), filename), content, 0o600)
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assert: func(t *testing.T, stream io.ReadCloser, size int, cachePath string) {
				t.Helper()

				defer stream.Close()
				gotContent, err := io.ReadAll(stream)
				require.NoError(t, err)

				require.Equal(t, content, gotContent)
				require.Equal(t, len(content), size)

				assertFile(t, filepath.Join(cachePath, id.String(), filename), len(content))
			},
		},

		{
			name: "error - filename is directory",

			setupCache: func(t *testing.T, cachePath string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(cachePath, id.String()), 0o700)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(cachePath, id.String(), filename), content, 0o200)
				require.NoError(t, err)
			},

			assertErr: require.Error,
			assert: func(t *testing.T, stream io.ReadCloser, size int, cachePath string) {
				t.Helper()

				require.Nil(t, stream)
				require.Zero(t, size)
			},
		},
		{
			name: "error - source.GetUpdateFileByFilename",

			sourceGetUpdateFileByFilenameErr: boom.Error,
			setupCache: func(t *testing.T, cachePath string) {
				t.Helper()
			},

			assertErr: boom.ErrorIs,
			assert: func(t *testing.T, stream io.ReadCloser, size int, cachePath string) {
				t.Helper()

				require.Nil(t, stream)
				require.Zero(t, size)
			},
		},
		{
			name: "error - Mkdir",

			setupCache: func(t *testing.T, cachePath string) {
				t.Helper()

				err := os.MkdirAll(filepath.Join(cachePath), 0o700)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(cachePath, id.String()), content, 0o600)
				require.NoError(t, err)
			},

			assertErr: require.Error,
			assert: func(t *testing.T, stream io.ReadCloser, size int, cachePath string) {
				t.Helper()

				require.Nil(t, stream)
				require.Zero(t, size)
			},
		},
		{
			name: "error - reader error",

			sourceGetUpdateFileByFilenameReadCloser: io.NopCloser(iotest.ErrReader(boom.Error)),
			sourceGetUpdateFileByFilenameSize:       len(content),
			setupCache: func(t *testing.T, path string) {
				t.Helper()
			},

			assertErr: require.NoError,
			assert: func(t *testing.T, stream io.ReadCloser, size int, cachePath string) {
				t.Helper()

				defer stream.Close()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &mock.UpdateSourcePortMock{
				GetUpdateFileByFilenameFunc: func(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
					return tc.sourceGetUpdateFileByFilenameReadCloser, tc.sourceGetUpdateFileByFilenameSize, tc.sourceGetUpdateFileByFilenameErr
				},
			}

			tmpDir := t.TempDir()

			tc.setupCache(t, tmpDir)

			f := filecache.New(source, tmpDir)

			stream, size, err := f.GetUpdateFileByFilename(context.Background(), provisioning.Update{
				UUID: id,
			}, filename)

			tc.assertErr(t, err)
			tc.assert(t, stream, size, tmpDir)
		})
	}
}

func assertFile(t *testing.T, path string, size int) {
	t.Helper()

	fi, err := os.Stat(path)
	require.NoError(t, err)

	require.Equal(t, size, int(fi.Size()))
}

func TestFilecache_ForgetUpdate(t *testing.T) {
	id := uuid.MustParse(`f9197021-8034-4b9d-b026-05be57ed4e66`)

	tests := []struct {
		name                  string
		sourceForgetUpdateErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:                  "error - source.ForgetUpdate",
			sourceForgetUpdateErr: boom.Error,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &mock.UpdateSourcePortMock{
				ForgetUpdateFunc: func(ctx context.Context, update provisioning.Update) error {
					return tc.sourceForgetUpdateErr
				},
			}

			tmpDir := t.TempDir()

			f := filecache.New(source, tmpDir)

			err := f.ForgetUpdate(context.Background(), provisioning.Update{
				UUID: id,
			})

			tc.assertErr(t, err)
		})
	}
}
