package local

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
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/signature"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestLocal_GetLatest(t *testing.T) {
	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)

		assertErr require.ErrorAssertionFunc
		updates   provisioning.Updates
	}{
		{
			name: "success - empty",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
			},

			assertErr: require.NoError,
			updates:   provisioning.Updates{},
		},
		{
			name: "success",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				writeUpdate(t, destDir, "0", provisioning.Update{
					Origin:      "testdata",
					Version:     "1",
					PublishedAt: time.Date(2025, 5, 21, 9, 3, 24, 0, time.UTC),
					Channel:     "daily",
					Severity:    api.UpdateSeverityNone,
				})
			},

			assertErr: require.NoError,
			updates: provisioning.Updates{
				provisioning.Update{
					UUID:        uuid.MustParse(`8be8344e-64e0-57f5-a7f6-a866231eda68`),
					Origin:      "testdata (local)",
					Version:     "1",
					PublishedAt: time.Date(2025, 5, 21, 9, 3, 24, 0, time.UTC),
					Channel:     "daily",
					Severity:    api.UpdateSeverityNone,
					Changelog:   `changelog`,
				},
			},
		},
		{
			name: "success - limit",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				writeUpdate(t, destDir, "0", provisioning.Update{
					Origin:      "testdata",
					Version:     "1",
					PublishedAt: time.Date(2025, 5, 21, 9, 3, 24, 0, time.UTC),
					Channel:     "daily",
					Severity:    api.UpdateSeverityNone,
				})

				writeUpdate(t, destDir, "1", provisioning.Update{
					Origin:      "testdata",
					Version:     "0",
					PublishedAt: time.Date(2024, 0, 0, 0, 0, 0, 0, time.UTC), // older file, is not returned
					Channel:     "daily",
					Severity:    api.UpdateSeverityNone,
				})
			},

			assertErr: require.NoError,
			updates: provisioning.Updates{
				provisioning.Update{
					UUID:        uuid.MustParse(`8be8344e-64e0-57f5-a7f6-a866231eda68`),
					Origin:      "testdata (local)",
					Version:     "1",
					PublishedAt: time.Date(2025, 5, 21, 9, 3, 24, 0, time.UTC),
					Channel:     "daily",
					Severity:    api.UpdateSeverityNone,
					Changelog:   `changelog`,
				},
			},
		},
		{
			name: "success - invalid update directory is skipped",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				writeUpdate(t, destDir, "0", provisioning.Update{
					Origin:      "testdata",
					Version:     "1",
					PublishedAt: time.Date(2025, 5, 21, 9, 3, 24, 0, time.UTC),
					Channel:     "daily",
					Severity:    api.UpdateSeverityNone,
				})

				err := os.MkdirAll(filepath.Join(destDir, "invalid"), 0o700) // invalid update directory, does not have update.json
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			updates: provisioning.Updates{
				provisioning.Update{
					UUID:        uuid.MustParse(`8be8344e-64e0-57f5-a7f6-a866231eda68`),
					Origin:      "testdata (local)",
					Version:     "1",
					PublishedAt: time.Date(2025, 5, 21, 9, 3, 24, 0, time.UTC),
					Channel:     "daily",
					Severity:    api.UpdateSeverityNone,
					Changelog:   `changelog`,
				},
			},
		},
		{
			name: "error - storage directory does not exist",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				err := os.RemoveAll(destDir)
				require.NoError(t, err)
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			update, err := New(tmpDir, nil)
			require.NoError(t, err)
			tc.setupTmpDir(t, tmpDir)

			// Run test
			gotUpdates, err := update.GetLatest(context.Background(), 1)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.updates, gotUpdates)
		})
	}
}

func TestLocal_GetUpdateAllFiles(t *testing.T) {
	tests := []struct {
		name        string
		setupTmpDir func(t *testing.T, destDir string)
		update      provisioning.Update

		assertErr   require.ErrorAssertionFunc
		updateFiles provisioning.UpdateFiles
	}{
		{
			name: "success",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
				writeUpdate(t, destDir, uuid.UUID{}.String(), provisioning.Update{
					Severity: api.UpdateSeverityNone,
					Files: provisioning.UpdateFiles{
						provisioning.UpdateFile{
							Filename:  "dummy.txt",
							Size:      0,
							Sha256:    "0",
							Component: api.UpdateFileComponentDebug,
							Type:      api.UpdateFileTypeImageManifest,
							URL:       "http://dummy.org/dummy.txt",
						},
					},
				})
			},
			update: provisioning.Update{
				UUID: uuid.UUID{},
			},

			assertErr: require.NoError,
			updateFiles: provisioning.UpdateFiles{
				provisioning.UpdateFile{
					Filename:  "dummy.txt",
					Size:      0,
					Sha256:    "0",
					Component: api.UpdateFileComponentDebug,
					Type:      api.UpdateFileTypeImageManifest,
					URL:       "http://dummy.org/dummy.txt",
				},
			},
		},
		{
			name: "error - update does not exist",
			setupTmpDir: func(t *testing.T, destDir string) {
				t.Helper()
			},
			update: provisioning.Update{
				UUID: uuid.UUID{},
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			update, err := New(tmpDir, nil)
			require.NoError(t, err)

			// Run test
			gotUpdateFiles, err := update.GetUpdateAllFiles(context.Background(), tc.update)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.updateFiles, gotUpdateFiles)
		})
	}
}

func writeUpdate(t *testing.T, destDir string, updateID string, update provisioning.Update) {
	t.Helper()

	err := os.MkdirAll(filepath.Join(destDir, updateID), 0o700)
	require.NoError(t, err)

	body, err := json.Marshal(update)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(destDir, updateID, "update.json"), body, 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(destDir, updateID, "changelog.txt"), []byte(`changelog`), 0o600)
	require.NoError(t, err)
}

func TestLocal_GetFileByFilename(t *testing.T) {
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
			update, err := New(tmpDir, nil)
			require.NoError(t, err)

			// Run test
			gotReader, _, err := update.GetUpdateFileByFilename(context.Background(), tc.update, tc.filename)

			// Assert
			tc.assertErr(t, err)
			tc.assertReader(t, gotReader)
		})
	}
}

func TestLocal_ForgetUpdate(t *testing.T) {
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
			update, err := New(tmpDir, nil)
			require.NoError(t, err)

			// Run test
			err = update.ForgetUpdate(context.Background(), tc.update)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

type testLocalAdd struct {
	name            string
	tarContentFiles string
	updateManifest  provisioning.Update
	setupTmpDir     func(t *testing.T, tmpDir string)

	assertErr    require.ErrorAssertionFunc
	assertUpdate func(t *testing.T, tmpDir string, update *provisioning.Update)
}

//go:embed testdata
var testdataFS embed.FS

const changelog = `This is the changelog`

func TestLocal_Add(t *testing.T) {
	tests := []testLocalAdd{
		{
			name:            "success",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Origin:      "testdata",
				Version:     "1",
				PublishedAt: time.Date(2025, 5, 21, 7, 25, 37, 0, time.UTC),
				Severity:    api.UpdateSeverityNone,
				Channel:     "daily",
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "file2.txt",
						Size:      fileSize(t, "testdata/success/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/file2.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: require.NoError,
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
				wantUUID := uuidFromUpdate(provisioning.Update{
					Origin:      "testdata (local)",
					Version:     "1",
					Channel:     "daily",
					PublishedAt: time.Date(2025, 5, 21, 7, 25, 37, 0, time.UTC),
				}).String()

				require.Equal(t, wantUUID, update.UUID.String())
				require.Len(t, update.Files, 2)
				require.Equal(t, api.UpdateFileComponentDebug, update.Files[0].Component)
				require.Equal(t, api.UpdateFileTypeImageManifest, update.Files[0].Type)

				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "update.json")))
				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "update.json.sig")))
				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "changelog.txt")))
				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "file1.txt")))
				require.True(t, file.PathExists(filepath.Join(tmpDir, wantUUID, "file2.txt")))
			},
		},
		{
			name:            "success - update already present",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Origin:   "testdata",
				Severity: api.UpdateSeverityNone,
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
				wantUUID := uuidFromUpdate(provisioning.Update{
					Origin: "testdata (local)",
				}).String()

				err := os.MkdirAll(filepath.Join(tmpDir, wantUUID), 0o700) // target directory for update already exists
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
				wantUUID := uuidFromUpdate(provisioning.Update{
					Origin: "testdata (local)",
				}).String()

				require.Equal(t, wantUUID, update.UUID.String())
			},
		},
		{
			name:            "error - file size mismatch",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Severity: api.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt") - 1, // filesize modified
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "file2.txt",
						Size:      fileSize(t, "testdata/success/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/file2.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
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
				Severity: api.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file2.txt"), // invalid sha256, file2 instead of file1
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "file2.txt",
						Size:      fileSize(t, "testdata/success/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/file2.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(t, err, `Invalid archive, file sha256 mismatch for file "file1.txt"`)
			},
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
		{
			name:            "error - additional file present in tar",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Severity: api.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "file2.txt",
						Size:      fileSize(t, "testdata/success/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/file2.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
					provisioning.UpdateFile{
						Filename:  "file3.txt", // Additional file in the manifest, missing in the tar.
						Size:      fileSize(t, "testdata/success/file2.txt"),
						Sha256:    fileSha256(t, "testdata/success/file2.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(t, err, `Invalid archive, failed to open file "file3.txt" mentioned in manifest`)
			},
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
		{
			name:            "error - file missing in tar",
			tarContentFiles: "testdata/success",
			updateManifest: provisioning.Update{
				Severity: api.UpdateSeverityNone,
				Files: provisioning.UpdateFiles{
					provisioning.UpdateFile{
						Filename:  "file1.txt",
						Size:      fileSize(t, "testdata/success/file1.txt"),
						Sha256:    fileSha256(t, "testdata/success/file1.txt"),
						Component: api.UpdateFileComponentDebug,
						Type:      api.UpdateFileTypeImageManifest,
					},
					// file2.txt not in manifest
				},
			},
			setupTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				require.ErrorContains(t, err, `Invalid archive, files not mentioned in the manifest found: file2.txt`)
			},
			assertUpdate: func(t *testing.T, tmpDir string, update *provisioning.Update) {
				t.Helper()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tr := generateUpdateTar(t, tc)

			tmpDir := t.TempDir()
			tc.setupTmpDir(t, tmpDir)
			// TODO: Mock verifier to simulate different error cases
			update, err := New(tmpDir, signature.NewNoopVerifier())
			require.NoError(t, err)

			// Run test
			gotUpdate, err := update.Add(context.Background(), tr)

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

func generateUpdateTar(t *testing.T, tc testLocalAdd) *tar.Reader {
	t.Helper()

	inMemoryTar := &bytes.Buffer{}

	tw := tar.NewWriter(inMemoryTar)

	entries, err := testdataFS.ReadDir(tc.tarContentFiles)
	require.NoError(t, err)

	for _, entry := range entries {
		fi, err := entry.Info()
		require.NoError(t, err)

		err = tw.WriteHeader(&tar.Header{
			Name: entry.Name(),
			Size: fi.Size(),
		})
		require.NoError(t, err)
		body, err := testdataFS.ReadFile(filepath.Join(tc.tarContentFiles, entry.Name()))
		require.NoError(t, err)

		_, err = tw.Write(body)
		require.NoError(t, err)

		h := sha256.New()
		_, err = h.Write(body)
		require.NoError(t, err)
	}

	body, err := json.Marshal(tc.updateManifest)
	require.NoError(t, err)

	err = tw.WriteHeader(&tar.Header{
		Name: "update.json",
		Size: int64(len(body)),
	})
	require.NoError(t, err)

	_, err = tw.Write(body)
	require.NoError(t, err)

	// FIXME: calculate signature and sign update.json file correctly
	err = tw.WriteHeader(&tar.Header{
		Name: "update.json.sig",
		Size: 0,
	})
	require.NoError(t, err)

	err = tw.WriteHeader(&tar.Header{
		Name: "changelog.txt",
		Size: int64(len(changelog)),
	})
	require.NoError(t, err)
	_, err = tw.Write([]byte(changelog))
	require.NoError(t, err)

	err = tw.Close()
	require.NoError(t, err)

	return tar.NewReader(inMemoryTar)
}

func fileSize(t *testing.T, path string) int {
	t.Helper()

	f, err := testdataFS.Open(path)
	require.NoError(t, err)

	fi, err := f.Stat()
	require.NoError(t, err)

	return int(fi.Size())
}

func fileSha256(t *testing.T, path string) string {
	t.Helper()

	f, err := testdataFS.Open(path)
	require.NoError(t, err)

	body, err := io.ReadAll(f)
	require.NoError(t, err)

	h := sha256.New()
	_, err = h.Write(body)
	require.NoError(t, err)

	return hex.EncodeToString(h.Sum(nil))
}
