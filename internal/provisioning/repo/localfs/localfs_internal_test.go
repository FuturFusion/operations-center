package localfs

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/signature/signaturetest"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
)

func TestLocalfs_Get(t *testing.T) {
	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)
		update      provisioning.Update
		filename    string

		assertErr    require.ErrorAssertionFunc
		assertReader func(t *testing.T, r io.ReadCloser)
	}{
		{
			name: "success",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				updateID := uuid.UUID{}.String()

				err := os.MkdirAll(filepath.Join(destDir, updateID), 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, updateID, "file1.txt"), []byte(`file1 body`), 0o600)
				require.NoError(t, err)
			},
			update: provisioning.Update{
				UUID: uuid.UUID{},
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
				updateID := uuid.UUID{}.String()

				err := os.MkdirAll(filepath.Join(destDir, updateID), 0o700)
				require.NoError(t, err)
			},
			update: provisioning.Update{
				UUID: uuid.UUID{},
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
			lfs, err := New(tmpDir, "")
			require.NoError(t, err)

			// Run test
			gotReader, _, err := lfs.Get(context.Background(), tc.update, tc.filename)

			// Assert
			tc.assertErr(t, err)
			tc.assertReader(t, gotReader)
		})
	}
}

func TestLocalfs_Put(t *testing.T) {
	tests := []struct {
		name   string
		update provisioning.Update
		stream io.ReadCloser
		commit bool
		cancel bool

		assertErr       require.ErrorAssertionFunc
		assertCommitErr require.ErrorAssertionFunc
		assertCancelErr require.ErrorAssertionFunc
	}{
		{
			name:   "success - commit",
			stream: io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			commit: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: require.NoError,
		},
		{
			name:   "success - commit + cancel",
			stream: io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			commit: true,
			cancel: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: require.NoError,
		},
		{
			name:   "cancel",
			stream: io.NopCloser(bytes.NewBuffer([]byte("foobar"))),
			cancel: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: require.NoError,
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
		},
		{
			name:   "error - stream close error in cancel",
			stream: errCloser(bytes.NewBuffer([]byte("foobar")), boom.Error),
			cancel: true,

			assertErr:       require.NoError,
			assertCommitErr: require.NoError,
			assertCancelErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			lfs, err := New(tmpDir, "")
			require.NoError(t, err)

			// Run test
			commit, cancel, err := lfs.Put(context.Background(), tc.update, "file.name", tc.stream)

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
		update      provisioning.Update

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				updateID := uuid.UUID{}.String()

				err := os.MkdirAll(filepath.Join(destDir, updateID), 0o700)
				require.NoError(t, err)
			},
			update: provisioning.Update{
				UUID: uuid.UUID{},
			},

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs, err := New(tmpDir, "")
			require.NoError(t, err)

			// Run test
			err = lfs.Delete(context.Background(), tc.update)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestLocalfs_CleanupAll(t *testing.T) {
	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)
	}{
		{
			name: "success - empty",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
			},
		},
		{
			name: "success - with content",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				updateID := uuid.UUID{}.String()

				err := os.MkdirAll(filepath.Join(destDir, updateID), 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(destDir, updateID, "file1.txt"), []byte(`file1 body`), 0o600)
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			lfs, err := New(tmpDir, "")
			require.NoError(t, err)

			// Run test
			err = lfs.CleanupAll(context.Background())

			// Assert
			require.NoError(t, err)
		})
	}
}

type testLocalfsCreateFromArchive struct {
	name            string
	tarContentFiles string
	updateManifest  provisioning.Update
	setupTmpDir     func(t *testing.T, tmpDir string)

	assertErr    require.ErrorAssertionFunc
	assertUpdate func(t *testing.T, tmpDir string, update *provisioning.Update)
}

//go:embed testdata
var testdataFS embed.FS

func TestLocalfs_CreateFromArchive(t *testing.T) {
	tests := []testLocalfsCreateFromArchive{
		{
			name:            "success",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Origin:           "testdata",
				Version:          "1",
				PublishedAt:      time.Date(2025, 5, 21, 7, 25, 37, 0, time.UTC),
				Severity:         images.UpdateSeverityNone,
				UpstreamChannels: []string{"daily"},
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "subdir/file2.txt",
						Size:      fileSize(t, "testdata/success/subdir/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/subdir/file2.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename: "file3.txt", // file does not have file component set, will be skipped.
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: require.NoError,
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
				wantUUID := uuidFromUpdate(Update{
					Origin:  "testdata (local)",
					Version: "1",
				}).String()

				require.Equal(t, wantUUID, update.UUID.String())
				require.Len(t, update.Files, 2)
				require.Equal(t, images.UpdateFileComponentDebug, update.Files[0].Component)
				require.Equal(t, images.UpdateFileTypeImageManifest, update.Files[0].Type)

				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "update.sjson")))
				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "file1.txt")))
				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "subdir/file2.txt")))
			},
		},
		{
			name:            "success - additional file present in manifest but missing in tar",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Severity: images.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "subdir/file2.txt",
						Size:      fileSize(t, "testdata/success/subdir/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/subdir/file2.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "file3.txt", // Additional file in the manifest, missing in the tar.
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: require.NoError,
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
		{
			name:            "success - additional file present in tar but missing in manifest",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Severity: images.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
					// file2.txt not in manifest
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: require.NoError,
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
		{
			name:            "success - update already present",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Origin:  "testdata",
				Version: "1",
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
				wantUUID := uuidFromUpdate(Update{
					Origin:  "testdata (local)",
					Version: "1",
				}).String()

				err := os.MkdirAll(filepath.Join(tmpDir, wantUUID), 0o700) // target directory for update already exists
				require.NoError(t, err)
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Update already existing")
			},
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
		{
			name:            "error - file size mismatch",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Severity: images.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt") - 1, // filesize modified
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "subdir/file2.txt",
						Size:      fileSize(t, "testdata/success/subdir/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/subdir/file2.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				t.Helper()
				require.ErrorContains(t, err, "Invalid archive, file size mismatch for file")
			},
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
		{
			name:            "error - file sha256 mismatch",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Severity: images.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/subdir/file2.txt"), // invalid sha256, file2 instead of file1
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "subdir/file2.txt",
						Size:      fileSize(t, "testdata/success/subdir/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/subdir/file2.txt"),
						Component: images.UpdateFileComponentDebug,
						Type:      images.UpdateFileTypeImageManifest,
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(t, err, `Invalid archive, file sha256 mismatch for file "file1.txt"`)
			},
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			caCert, cert, key := signaturetest.GenerateCertChain(t)
			tr := generateUpdateTar(t, tc, cert, key)

			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			// TODO: Mock verifier to simulate different error cases
			lfs, err := New(tmpDir, string(caCert))
			require.NoError(t, err)

			// Run test
			gotUpdate, err := lfs.CreateFromArchive(context.Background(), tr)

			// Assert
			tc.assertErr(t, err)
			tc.assertUpdate(t, tmpDir, gotUpdate)

			// Ensure, the temporary folder is gone.
			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)
			for _, entry := range entries {
				require.NotContains(t, entry.Name(), tmpUpdateDirPrefix)
			}
		})
	}
}

func generateUpdateTar(t *testing.T, tc testLocalfsCreateFromArchive, cert []byte, key []byte) *tar.Reader {
	t.Helper()

	inMemoryTar := &bytes.Buffer{}

	tw := tar.NewWriter(inMemoryTar)

	addFilesRecursively(t, tc, tc.tarContentFiles, "", tw)

	body, err := json.Marshal(tc.updateManifest)
	require.NoError(t, err)

	signedBody := signaturetest.SignContent(t, cert, key, body)

	err = tw.WriteHeader(&tar.Header{
		Name: "update.sjson",
		Size: int64(len(signedBody)),
	})
	require.NoError(t, err)

	_, err = tw.Write(signedBody)
	require.NoError(t, err)

	err = tw.Close()
	require.NoError(t, err)

	return tar.NewReader(inMemoryTar)
}

func addFilesRecursively(t *testing.T, tc testLocalfsCreateFromArchive, dir string, pathPrefix string, tw *tar.Writer) {
	t.Helper()

	entries, err := testdataFS.ReadDir(dir)
	require.NoError(t, err)

	for _, entry := range entries {
		fi, err := entry.Info()
		require.NoError(t, err)

		if entry.IsDir() {
			err = tw.WriteHeader(&tar.Header{
				Name:     entry.Name(),
				Typeflag: tar.TypeDir,
			})
			require.NoError(t, err)

			addFilesRecursively(t, tc, path.Join(dir, entry.Name()), path.Join(pathPrefix, entry.Name()), tw)
			continue
		}

		err = tw.WriteHeader(&tar.Header{
			Name: path.Join(pathPrefix, entry.Name()),
			Size: fi.Size(),
		})
		require.NoError(t, err)

		body, err := testdataFS.ReadFile(filepath.Join(tc.tarContentFiles, path.Join(pathPrefix, entry.Name())))
		require.NoError(t, err)

		_, err = tw.Write(body)
		require.NoError(t, err)

		h := sha256.New()
		_, err = h.Write(body)
		require.NoError(t, err)
	}
}

func fileSize(t *testing.T, filePath string) int {
	t.Helper()

	f, err := testdataFS.Open(filePath)
	require.NoError(t, err)

	fi, err := f.Stat()
	require.NoError(t, err)

	return int(fi.Size())
}

func fileSha256(t *testing.T, filePath string) string {
	t.Helper()

	f, err := testdataFS.Open(filePath)
	require.NoError(t, err)

	body, err := io.ReadAll(f)
	require.NoError(t, err)

	h := sha256.New()
	_, err = h.Write(body)
	require.NoError(t, err)

	return hex.EncodeToString(h.Sum(nil))
}
