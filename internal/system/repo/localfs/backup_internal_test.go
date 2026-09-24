package localfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/util/file"
)

// usageWithAvailable reports a file system of 1 GiB with the given space available.
func usageWithAvailable(available uint64) usageFunc {
	return func() (file.UsageInformation, error) {
		return file.UsageInformation{
			TotalSpaceBytes:     1 << 30,
			AvailableSpaceBytes: available,
		}, nil
	}
}

func writeStateFiles(t *testing.T, dir string, content string) {
	t.Helper()

	for _, name := range []string{"config.yml", "server.crt", "server.key", "client.crt", "client.key", "local.db"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
		require.NoError(t, err)
	}

	for _, name := range []string{"artifacts/a/file", "images/i/file", "updates/u/file"} {
		err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o700)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
		require.NoError(t, err)
	}
}

func TestBackup_WriteAndExtract(t *testing.T) {
	varDir := t.TempDir()
	writeStateFiles(t, varDir, "state")

	snapshotDir := t.TempDir()
	err := os.WriteFile(filepath.Join(snapshotDir, "local.db"), []byte("snapshot"), 0o600)
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	err = writeBackup(buf, varDir, snapshotDir, system.BackupOptions{})
	require.NoError(t, err)

	extractDir := t.TempDir()
	err = extractBackup(t.Context(), buf, extractDir, usageWithAvailable(1<<30))
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(extractDir, "artifacts", "a", "file"))
	require.NoDirExists(t, filepath.Join(extractDir, "images"))
	require.NoDirExists(t, filepath.Join(extractDir, "updates"))
	require.NoFileExists(t, filepath.Join(extractDir, backupManifestFilename))

	content, err := os.ReadFile(filepath.Join(extractDir, "local.db"))
	require.NoError(t, err)
	require.Equal(t, "snapshot", string(content))

	info, err := os.Stat(filepath.Join(extractDir, "server.key"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestBackup_WriteOptionalEntries(t *testing.T) {
	varDir := t.TempDir()
	writeStateFiles(t, varDir, "state")

	buf := &bytes.Buffer{}
	err := writeBackup(buf, varDir, varDir, system.BackupOptions{Images: true})
	require.NoError(t, err)

	extractDir := t.TempDir()
	err = extractBackup(t.Context(), buf, extractDir, usageWithAvailable(1<<30))
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(extractDir, "images", "i", "file"))
	require.NoDirExists(t, filepath.Join(extractDir, "updates"))
}

func TestBackup_ExtractRejectsForeignEntries(t *testing.T) {
	for _, name := range []string{"../escape", "unknown.txt", "artifacts/../../escape"} {
		t.Run(name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			gzw := gzip.NewWriter(buf)
			tw := tar.NewWriter(gzw)

			err := writeManifest(tw, backupManifest{FormatVersion: backupFormatVersion})
			require.NoError(t, err)

			err = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: 1, Typeflag: tar.TypeReg})
			require.NoError(t, err)

			_, err = tw.Write([]byte("x"))
			require.NoError(t, err)

			require.NoError(t, tw.Close())
			require.NoError(t, gzw.Close())

			err = extractBackup(t.Context(), buf, t.TempDir(), usageWithAvailable(1<<30))
			require.Error(t, err)
		})
	}
}

func TestBackup_ExtractRejectsUnsupportedManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest *backupManifest
	}{
		{
			name: "no manifest",
		},
		{
			name:     "format version missing",
			manifest: &backupManifest{},
		},
		{
			name:     "format version newer",
			manifest: &backupManifest{FormatVersion: backupFormatVersion + 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			gzw := gzip.NewWriter(buf)
			tw := tar.NewWriter(gzw)

			if tc.manifest != nil {
				err := writeManifest(tw, *tc.manifest)
				require.NoError(t, err)
			}

			err := tw.WriteHeader(&tar.Header{Name: "config.yml", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg})
			require.NoError(t, err)

			_, err = tw.Write([]byte("x"))
			require.NoError(t, err)

			require.NoError(t, tw.Close())
			require.NoError(t, gzw.Close())

			extractDir := t.TempDir()
			err = extractBackup(t.Context(), buf, extractDir, usageWithAvailable(1<<30))
			require.ErrorIs(t, err, domain.ErrInvalidArgument)

			// Nothing is extracted from an unsupported backup.
			require.NoFileExists(t, filepath.Join(extractDir, "config.yml"))
		})
	}
}

func TestBackup_ExtractRejectsCorruptArchive(t *testing.T) {
	varDir := t.TempDir()
	writeStateFiles(t, varDir, "state")

	buf := &bytes.Buffer{}
	err := writeBackup(buf, varDir, varDir, system.BackupOptions{})
	require.NoError(t, err)

	// Corrupt the checksum in the gzip trailer.
	archive := buf.Bytes()
	archive[len(archive)-8] ^= 0xff

	err = extractBackup(t.Context(), bytes.NewReader(archive), t.TempDir(), usageWithAvailable(1<<30))
	require.Error(t, err)
}

func TestBackup_ExtractRejectsWithoutSpace(t *testing.T) {
	varDir := t.TempDir()
	writeStateFiles(t, varDir, "state")

	buf := &bytes.Buffer{}
	err := writeBackup(buf, varDir, varDir, system.BackupOptions{})
	require.NoError(t, err)

	// Less than 10% of the total space is available.
	err = extractBackup(t.Context(), buf, t.TempDir(), usageWithAvailable(1<<20))
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}

func TestBackup_ApplyRestore(t *testing.T) {
	varDir := t.TempDir()
	writeStateFiles(t, varDir, "old")

	err := os.WriteFile(filepath.Join(varDir, "local.db-journal"), []byte("old"), 0o600)
	require.NoError(t, err)

	stagedDir := filepath.Join(varDir, restoreDirName, "staged")
	err = os.MkdirAll(stagedDir, 0o700)
	require.NoError(t, err)

	writeStateFiles(t, stagedDir, "new")
	err = os.RemoveAll(filepath.Join(stagedDir, "images"))
	require.NoError(t, err)

	err = os.RemoveAll(filepath.Join(stagedDir, "updates"))
	require.NoError(t, err)

	// Apply the staged restore.
	err = NewBackup(varDir).ApplyRestore()
	require.NoError(t, err)

	require.True(t, NewBackup(varDir).IsRestoreApplied())
	requireFileContent(t, filepath.Join(varDir, "config.yml"), "new")
	require.NoFileExists(t, filepath.Join(varDir, "local.db-journal"))

	// The updates are kept, since the backup does not include them.
	requireFileContent(t, filepath.Join(varDir, "updates", "u", "file"), "old")

	// The images are removed, the database of a backup without images does not know them.
	require.NoDirExists(t, filepath.Join(varDir, "images"))

	// A new restore is refused, it would remove the state needed to roll back.
	_, err = NewBackup(varDir).Extract(t.Context(), strings.NewReader(""))
	require.ErrorIs(t, err, domain.ErrOperationNotPermitted)

	// The daemon did not start successfully, roll back.
	err = NewBackup(varDir).ApplyRestore()
	require.ErrorIs(t, err, system.ErrRestoreRolledBack)

	require.False(t, NewBackup(varDir).IsRestoreApplied())
	requireFileContent(t, filepath.Join(varDir, "config.yml"), "old")
	requireFileContent(t, filepath.Join(varDir, "local.db-journal"), "old")
	requireFileContent(t, filepath.Join(varDir, "images", "i", "file"), "old")
	requireFileContent(t, filepath.Join(varDir, "updates", "u", "file"), "old")
	require.NoDirExists(t, filepath.Join(varDir, restoreDirName))

	// Nothing staged, nothing to do.
	err = NewBackup(varDir).ApplyRestore()
	require.NoError(t, err)
	requireFileContent(t, filepath.Join(varDir, "config.yml"), "old")
}

func TestBackup_ApplyRestoreRollbackRetried(t *testing.T) {
	varDir := t.TempDir()
	writeStateFiles(t, varDir, "old")

	stagedDir := filepath.Join(varDir, restoreDirName, "staged")
	err := os.MkdirAll(stagedDir, 0o700)
	require.NoError(t, err)

	writeStateFiles(t, stagedDir, "new")

	err = NewBackup(varDir).ApplyRestore()
	require.NoError(t, err)

	// A roll back, which has been interrupted after putting back the first entry.
	err = os.Remove(filepath.Join(varDir, "config.yml"))
	require.NoError(t, err)

	err = os.Rename(filepath.Join(varDir, restoreDirName, "previous", "config.yml"), filepath.Join(varDir, "config.yml"))
	require.NoError(t, err)

	err = NewBackup(varDir).ApplyRestore()
	require.ErrorIs(t, err, system.ErrRestoreRolledBack)

	requireFileContent(t, filepath.Join(varDir, "config.yml"), "old")
	requireFileContent(t, filepath.Join(varDir, "updates", "u", "file"), "old")
	require.NoDirExists(t, filepath.Join(varDir, restoreDirName))
}

func TestBackup_CompleteRestore(t *testing.T) {
	varDir := t.TempDir()
	writeStateFiles(t, varDir, "old")

	stagedDir := filepath.Join(varDir, restoreDirName, "staged")
	err := os.MkdirAll(stagedDir, 0o700)
	require.NoError(t, err)

	writeStateFiles(t, stagedDir, "new")

	err = NewBackup(varDir).ApplyRestore()
	require.NoError(t, err)

	err = NewBackup(varDir).CompleteRestore()
	require.NoError(t, err)

	require.False(t, NewBackup(varDir).IsRestoreApplied())
	require.NoDirExists(t, filepath.Join(varDir, restoreDirName))

	// A restart after the completed restore keeps the restored state.
	err = NewBackup(varDir).ApplyRestore()
	require.NoError(t, err)
	requireFileContent(t, filepath.Join(varDir, "config.yml"), "new")
	requireFileContent(t, filepath.Join(varDir, "updates", "u", "file"), "new")
}

func requireFileContent(t *testing.T, filename string, want string) {
	t.Helper()

	content, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, want, string(content))
}
