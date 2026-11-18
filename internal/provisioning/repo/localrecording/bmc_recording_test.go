package localrecording_test

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/localrecording"
	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
	"github.com/FuturFusion/operations-center/internal/version"
	"github.com/FuturFusion/operations-center/shared/api"
)

var (
	startedAt = time.Date(2026, 10, 27, 8, 15, 0, 0, time.UTC)
	now       = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
)

const recordingID = "20261027T081500Z"

func newTestRepo(t *testing.T) (provisioning.BMCRecordingRepo, string) {
	t.Helper()

	storageDir := filepath.Join(t.TempDir(), "bmc-recordings")
	require.NoError(t, os.Mkdir(storageDir, 0o700))

	repo := localrecording.New(fsroot.Open(t, storageDir), localrecording.WithNow(func() time.Time { return now }))

	return repo, storageDir
}

func testServer() provisioning.Server {
	return provisioning.Server{
		Name: "server01",
		BMCConfig: api.BMCConfig{
			Endpoint: "https://bmc.example.com",
			Username: "admin",
			Password: "secret-password",
		},
		BMCData: api.BMCData{
			ServerSerialNumber: "SN123456",
			ServerModel:        "Model X",
		},
	}
}

func testDeployment(startedAt time.Time) provisioning.ServerDeployment {
	return provisioning.ServerDeployment{
		State: api.ServerDeploymentStateApplyBIOS,
		Request: provisioning.ServerDeploymentRequest{
			TokenUUID: uuid.MustParse("8f6c3d1a-2b4e-4c9a-9f7d-1a2b3c4d5e6f"),
			Seed:      "seed01",
			ImageType: api.ImageTypeISO,
			Channel:   "stable",
		},
		MediaURL:  "https://oc.example.com/media/seed01.iso",
		StartedAt: startedAt,
	}
}

func readLines(t *testing.T, path string) []api.BMCRecordingExchange {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var exchanges []api.BMCRecordingExchange

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		var exchange api.BMCRecordingExchange
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &exchange))

		exchanges = append(exchanges, exchange)
	}

	return exchanges
}

func readManifestFile(t *testing.T, path string) api.BMCRecordingManifest {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var manifest api.BMCRecordingManifest
	require.NoError(t, json.Unmarshal(content, &manifest))

	return manifest
}

func TestBMCRecording_Open(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)
	dir := filepath.Join(storageDir, "server01", recordingID)

	// Run test
	session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)

	defer func() { _ = session.Finalize(t.Context(), testDeployment(startedAt)) }()

	// Assert
	require.FileExists(t, filepath.Join(dir, "exchanges.jsonl"))

	content, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)
	require.NotContains(t, string(content), "secret-password", "the BMC password never enters the manifest")
	require.NotContains(t, string(content), `"admin"`, "the BMC username never enters the manifest")

	require.Equal(t, api.BMCRecordingManifest{
		FormatVersion:           api.BMCRecordingFormatVersion,
		Server:                  "server01",
		RecordingID:             recordingID,
		OperationsCenterVersion: version.Version,
		StartedAt:               startedAt,
		Request:                 testDeployment(startedAt).Request.ToAPI(),
		BMCData:                 testServer().BMCData,
		BMCEndpoint:             "https://bmc.example.com",
		MediaURLs:               []string{"https://oc.example.com/media/seed01.iso"},
	}, readManifestFile(t, filepath.Join(dir, "manifest.json")))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "the recording directory is private")
}

func TestBMCRecording_RecordAndFinalize(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)
	dir := filepath.Join(storageDir, "server01", recordingID)

	session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)

	deployment := testDeployment(startedAt)
	deployment.State = api.ServerDeploymentStateCompleted
	deployment.SecureBootMediaURL = "https://oc.example.com/media/secure-boot.iso"
	deployment.History = []api.ServerDeploymentStep{{State: api.ServerDeploymentStateApplyBIOS, EnteredAt: startedAt}}

	// Run test
	session.Record(t.Context(), api.BMCRecordingExchange{Method: "GET", Status: 200})
	session.Record(t.Context(), api.BMCRecordingExchange{Method: "PATCH", Status: 200, ResponseTruncated: true})
	statsBeforeFinalize := session.Stats()

	err = session.Finalize(t.Context(), deployment)
	require.NoError(t, err)

	session.Record(t.Context(), api.BMCRecordingExchange{Method: "GET", Status: 200})
	errAgain := session.Finalize(t.Context(), deployment)

	// Assert
	exchanges := readLines(t, filepath.Join(dir, "exchanges.jsonl"))
	require.Len(t, exchanges, 2, "the exchange after finalize is dropped")
	require.Equal(t, int64(1), exchanges[0].Sequence)
	require.Equal(t, int64(2), exchanges[1].Sequence)

	info, err := os.Stat(filepath.Join(dir, "exchanges.jsonl"))
	require.NoError(t, err)
	require.Equal(t, provisioning.BMCRecordingStats{Exchanges: 2, Bytes: info.Size(), TruncatedExchanges: 1}, statsBeforeFinalize)
	require.Equal(t, statsBeforeFinalize, session.Stats())
	require.NoError(t, errAgain, "a second finalize is harmless")

	manifest := readManifestFile(t, filepath.Join(dir, "manifest.json"))
	require.Equal(t, now, manifest.FinishedAt, "without a finish time of the deployment the clock is used")
	require.Equal(t, api.ServerDeploymentStateCompleted, manifest.FinalState)
	require.Equal(t, deployment.History, manifest.History)
	require.Equal(t, []string{"https://oc.example.com/media/seed01.iso", "https://oc.example.com/media/secure-boot.iso"}, manifest.MediaURLs)
	require.Equal(t, int64(2), manifest.Exchanges)
	require.Equal(t, info.Size(), manifest.Bytes)
	require.Equal(t, int64(1), manifest.TruncatedExchanges)
}

func TestBMCRecording_Reopen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		corrupt func(t *testing.T, path string)

		wantStats provisioning.BMCRecordingStats
	}{
		{
			name:    "continues after a clean finalize",
			corrupt: func(*testing.T, string) {},

			wantStats: provisioning.BMCRecordingStats{Exchanges: 3, TruncatedExchanges: 1},
		},
		{
			name: "drops a partial last line",
			corrupt: func(t *testing.T, path string) {
				t.Helper()

				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
				require.NoError(t, err)

				_, err = f.WriteString(`{"sequence":4,"method":"GE`)
				require.NoError(t, err)
				require.NoError(t, f.Close())
			},

			wantStats: provisioning.BMCRecordingStats{Exchanges: 3, TruncatedExchanges: 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo, storageDir := newTestRepo(t)
			path := filepath.Join(storageDir, "server01", recordingID, "exchanges.jsonl")

			first, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
			require.NoError(t, err)

			first.Record(t.Context(), api.BMCRecordingExchange{Method: "GET"})
			first.Record(t.Context(), api.BMCRecordingExchange{Method: "GET", RequestTruncated: true})
			first.Record(t.Context(), api.BMCRecordingExchange{Method: "GET"})
			require.NoError(t, first.Finalize(t.Context(), testDeployment(startedAt)))

			info, err := os.Stat(path)
			require.NoError(t, err)

			tc.corrupt(t, path)

			// Run test
			second, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
			require.NoError(t, err)

			statsOnReopen := second.Stats()
			second.Record(t.Context(), api.BMCRecordingExchange{Method: "PATCH"})
			require.NoError(t, second.Finalize(t.Context(), testDeployment(startedAt)))

			// Assert
			wantStats := tc.wantStats
			wantStats.Bytes = info.Size()
			require.Equal(t, wantStats, statsOnReopen, "the second session continues the counts")

			exchanges := readLines(t, path)
			require.Len(t, exchanges, 4)
			require.Equal(t, int64(4), exchanges[3].Sequence)
			require.Equal(t, "PATCH", exchanges[3].Method)
		})
	}
}

func TestBMCRecording_OpenSameSecond(t *testing.T) {
	t.Parallel()

	// Setup
	repo, _ := newTestRepo(t)

	first, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)
	require.NoError(t, first.Finalize(t.Context(), testDeployment(startedAt)))

	// Run test
	_, err = repo.Open(t.Context(), testServer(), testDeployment(startedAt.Add(500*time.Millisecond)))

	// Assert
	require.ErrorContains(t, err, "belongs to another deployment", "a deployment started in the same second does not reuse the recording")
}

func TestBMCRecording_ReopenAfterInvalidLine(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)
	path := filepath.Join(storageDir, "server01", recordingID, "exchanges.jsonl")

	first, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)

	first.Record(t.Context(), api.BMCRecordingExchange{Method: "GET"})
	require.NoError(t, first.Finalize(t.Context(), testDeployment(startedAt)))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)

	// A partial line that a later exchange was appended to.
	_, err = f.WriteString(`{"sequence":2,"method":"GE{"sequence":2,"method":"GET"}` + "\n" + `{"sequence":3,"method":"GET","response_truncated":true}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	info, err := os.Stat(path)
	require.NoError(t, err)

	// Run test
	second, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)

	stats := second.Stats()
	require.NoError(t, second.Finalize(t.Context(), testDeployment(startedAt)))

	// Assert
	require.Equal(t, provisioning.BMCRecordingStats{Exchanges: 2, Bytes: info.Size(), TruncatedExchanges: 1}, stats, "the invalid line is skipped, the lines after it are kept")

	after, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, info.Size(), after.Size(), "no line is cut")
}

func TestBMCRecording_Prune(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)

	// Run test
	for i := range config.BMCRecordingsPerServer + 1 {
		session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt.Add(time.Duration(i)*time.Hour)))
		require.NoError(t, err)
		require.NoError(t, session.Finalize(t.Context(), testDeployment(startedAt)))
	}

	// Assert
	entries, err := os.ReadDir(filepath.Join(storageDir, "server01"))
	require.NoError(t, err)
	require.Len(t, entries, config.BMCRecordingsPerServer)
	require.Equal(t, "20261027T091500Z", entries[0].Name(), "the oldest recording is removed")
}

func TestBMCRecording_PruneKeepsOpenRecording(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)

	for i := range config.BMCRecordingsPerServer {
		session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt.Add(time.Duration(i+1)*time.Hour)))
		require.NoError(t, err)
		require.NoError(t, session.Finalize(t.Context(), testDeployment(startedAt)))
	}

	// Run test
	session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)
	require.NoError(t, session.Finalize(t.Context(), testDeployment(startedAt)))

	// Assert
	entries, err := os.ReadDir(filepath.Join(storageDir, "server01"))
	require.NoError(t, err)
	require.Len(t, entries, config.BMCRecordingsPerServer)
	require.Equal(t, recordingID, entries[0].Name(), "the open recording is kept, even if it is the oldest")
	require.Equal(t, "20261027T101500Z", entries[1].Name(), "the oldest other recording is removed")
}

func TestBMCRecording_GetAll(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)

	for _, started := range []time.Time{startedAt, startedAt.Add(time.Hour)} {
		session, err := repo.Open(t.Context(), testServer(), testDeployment(started))
		require.NoError(t, err)
		require.NoError(t, session.Finalize(t.Context(), testDeployment(started)))
	}

	require.NoError(t, os.MkdirAll(filepath.Join(storageDir, "server01", "20261027T101500Z"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(storageDir, "server01", "not-a-recording"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(storageDir, "server01", "20261027T111500Z"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(storageDir, "server01", "20261027T111500Z", "manifest.json"), []byte("{"), 0o600))

	// Run test
	manifests, err := repo.GetAll(t.Context(), "server01")
	require.NoError(t, err)

	unknown, errUnknown := repo.GetAll(t.Context(), "server02")

	// Assert
	require.Len(t, manifests, 2, "directories without a readable manifest are skipped")
	require.Equal(t, "20261027T091500Z", manifests[0].RecordingID, "the newest recording comes first")
	require.Equal(t, recordingID, manifests[1].RecordingID)
	require.NoError(t, errUnknown)
	require.Empty(t, unknown)
}

func TestBMCRecording_GetByID(t *testing.T) {
	t.Parallel()

	// Setup
	repo, _ := newTestRepo(t)

	session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)
	require.NoError(t, session.Finalize(t.Context(), testDeployment(startedAt)))

	// Run test
	manifest, err := repo.GetByID(t.Context(), "server01", recordingID)
	require.NoError(t, err)

	_, errUnknown := repo.GetByID(t.Context(), "server01", "20261027T091500Z")

	// Assert
	require.Equal(t, recordingID, manifest.RecordingID)
	require.ErrorIs(t, errUnknown, domain.ErrNotFound)
}

func TestBMCRecording_GetByIDNewerFormat(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)

	dir := filepath.Join(storageDir, "server01", recordingID)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"format_version":99}`), 0o600))

	// Run test
	_, err := repo.GetByID(t.Context(), "server01", recordingID)

	// Assert
	require.ErrorContains(t, err, "format version 99 is newer", "a newer format is rejected")
}

func TestBMCRecording_GetFile(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)

	session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	require.NoError(t, err)

	session.Record(t.Context(), api.BMCRecordingExchange{Method: "GET"})
	require.NoError(t, session.Finalize(t.Context(), testDeployment(startedAt)))

	want, err := os.ReadFile(filepath.Join(storageDir, "server01", recordingID, "exchanges.jsonl"))
	require.NoError(t, err)

	// Run test
	rc, size, err := repo.GetFile(t.Context(), "server01", recordingID, "exchanges.jsonl")
	require.NoError(t, err)

	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)

	_, _, errDisallowed := repo.GetFile(t.Context(), "server01", recordingID, "../manifest.json")
	_, _, errUnknown := repo.GetFile(t.Context(), "server01", "20261027T091500Z", "manifest.json")

	// Assert
	require.Equal(t, want, got)
	require.Equal(t, int64(len(want)), size)
	require.ErrorAs(t, errDisallowed, new(domain.ErrValidation), "only the recording files are served")
	require.ErrorIs(t, errUnknown, domain.ErrNotFound)
}

func TestBMCRecording_GetArchive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rewrite *provisioning.BMCRecordingRewrite

		want func(original string) string
	}{
		{
			name:    "as recorded",
			rewrite: nil,

			want: func(original string) string { return original },
		},
		{
			name: "rewritten",
			rewrite: &provisioning.BMCRecordingRewrite{
				Replacements: []provisioning.BMCRecordingReplacement{
					{Old: "bmc.example.com", New: "bmc.invalid"},
				},
			},

			want: func(original string) string {
				return strings.ReplaceAll(original, "bmc.example.com", "bmc.invalid")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo, storageDir := newTestRepo(t)
			dir := filepath.Join(storageDir, "server01", recordingID)

			session, err := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
			require.NoError(t, err)

			session.Record(t.Context(), api.BMCRecordingExchange{Method: "GET", URL: "https://bmc.example.com/redfish/v1/"})
			session.Record(t.Context(), api.BMCRecordingExchange{Method: "GET", URL: "https://bmc.example.com/redfish/v1/Systems"})
			require.NoError(t, session.Finalize(t.Context(), testDeployment(startedAt)))

			// Run test
			rc, size, err := repo.GetArchive(t.Context(), "server01", recordingID, tc.rewrite)
			require.NoError(t, err)

			defer rc.Close()

			content, err := io.ReadAll(rc)
			require.NoError(t, err)

			// Assert
			require.Equal(t, len(content), size)

			archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
			require.NoError(t, err)
			require.Len(t, archive.File, 2)

			for _, f := range archive.File {
				original, err := os.ReadFile(filepath.Join(dir, f.Name))
				require.NoError(t, err)

				r, err := f.Open()
				require.NoError(t, err)

				got, err := io.ReadAll(r)
				require.NoError(t, err)
				require.NoError(t, r.Close())

				require.Contains(t, string(original), "bmc.example.com", "the file %s carries the value to rewrite", f.Name)
				require.Equal(t, tc.want(string(original)), string(got), "content of %s", f.Name)
			}
		})
	}
}

func TestBMCRecording_GetArchiveNotFound(t *testing.T) {
	t.Parallel()

	repo, _ := newTestRepo(t)

	_, _, err := repo.GetArchive(t.Context(), "server01", recordingID, nil)

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestBMCRecording_Delete(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)

	for _, started := range []time.Time{startedAt, startedAt.Add(time.Hour)} {
		session, err := repo.Open(t.Context(), testServer(), testDeployment(started))
		require.NoError(t, err)
		require.NoError(t, session.Finalize(t.Context(), testDeployment(started)))
	}

	// Run test
	errByID := repo.DeleteByID(t.Context(), "server01", recordingID)
	_, errAfterDeleteByID := os.Stat(filepath.Join(storageDir, "server01", recordingID))
	errByIDAgain := repo.DeleteByID(t.Context(), "server01", recordingID)

	errAll := repo.DeleteAll(t.Context(), "server01")
	_, errAfterDeleteAll := os.Stat(filepath.Join(storageDir, "server01"))
	errAllAgain := repo.DeleteAll(t.Context(), "server01")

	// Assert
	require.NoError(t, errByID)
	require.ErrorIs(t, errAfterDeleteByID, os.ErrNotExist)
	require.NoError(t, errByIDAgain, "deleting a missing recording is not an error")
	require.NoError(t, errAll)
	require.ErrorIs(t, errAfterDeleteAll, os.ErrNotExist)
	require.NoError(t, errAllAgain, "deleting the recordings of a server without any is not an error")
}

func TestBMCRecording_Validation(t *testing.T) {
	t.Parallel()

	calls := []struct {
		name string
		call func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string, id string) error
	}{
		{
			name: "GetByID",
			call: func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string, id string) error {
				t.Helper()

				_, err := repo.GetByID(t.Context(), serverName, id)

				return err
			},
		},
		{
			name: "GetFile",
			call: func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string, id string) error {
				t.Helper()

				_, _, err := repo.GetFile(t.Context(), serverName, id, "manifest.json")

				return err
			},
		},
		{
			name: "GetArchive",
			call: func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string, id string) error {
				t.Helper()

				_, _, err := repo.GetArchive(t.Context(), serverName, id, nil)

				return err
			},
		},
		{
			name: "DeleteByID",
			call: func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string, id string) error {
				t.Helper()

				return repo.DeleteByID(t.Context(), serverName, id)
			},
		},
	}

	serverOnlyCalls := []struct {
		name string
		call func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string) error
	}{
		{
			name: "Open",
			call: func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string) error {
				t.Helper()

				server := testServer()
				server.Name = serverName

				_, err := repo.Open(t.Context(), server, testDeployment(startedAt))

				return err
			},
		},
		{
			name: "GetAll",
			call: func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string) error {
				t.Helper()

				_, err := repo.GetAll(t.Context(), serverName)

				return err
			},
		},
		{
			name: "DeleteAll",
			call: func(t *testing.T, repo provisioning.BMCRecordingRepo, serverName string) error {
				t.Helper()

				return repo.DeleteAll(t.Context(), serverName)
			},
		},
	}

	invalidServerNames := []string{"", ".", "..", "../server01", "server01/x"}
	invalidIDs := []string{"", "..", "../" + recordingID, recordingID + "/x"}

	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			repo, _ := newTestRepo(t)

			for _, serverName := range invalidServerNames {
				err := c.call(t, repo, serverName, recordingID)
				require.ErrorAs(t, err, new(domain.ErrValidation), "server name %q", serverName)
			}

			for _, id := range invalidIDs {
				err := c.call(t, repo, "server01", id)
				require.ErrorAs(t, err, new(domain.ErrValidation), "recording ID %q", id)
			}
		})
	}

	for _, c := range serverOnlyCalls {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			repo, _ := newTestRepo(t)

			for _, serverName := range invalidServerNames {
				err := c.call(t, repo, serverName)
				require.ErrorAs(t, err, new(domain.ErrValidation), "server name %q", serverName)
			}
		})
	}
}

func TestBMCRecording_SymlinkOutsideOfRoot(t *testing.T) {
	t.Parallel()

	// Setup
	repo, storageDir := newTestRepo(t)
	outsideDir := t.TempDir()
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(storageDir, "server01")))

	// Run test
	_, errOpen := repo.Open(t.Context(), testServer(), testDeployment(startedAt))
	_, errGetAll := repo.GetAll(t.Context(), "server01")

	// Assert
	require.Error(t, errOpen, "a server directory pointing outside of the root is refused")
	require.Error(t, errGetAll, "a server directory pointing outside of the root is not listed")

	entries, err := os.ReadDir(outsideDir)
	require.NoError(t, err)
	require.Empty(t, entries, "nothing is written outside of the root")
}

func TestBMCRecording_GetArchiveAnonymized(t *testing.T) {
	t.Parallel()

	const operationsCenterAddress = "https://oc.corp.example.net:7443"

	server := provisioning.Server{
		Name:          "r640-03",
		ConnectionURL: "https://r640-03.corp.example.net:8443",
		BMCConfig: api.BMCConfig{
			Endpoint: "https://bmc.corp.example.net",
			Username: "bmcadmin",
			Password: "Sup3rS3cret!",
		},
		BMCData: api.BMCData{
			ServerSerialNumber: "SN123456",
			ServerSKU:          "R640SKU77",
			ServerAssetTag:     "R&D-07",
			ServerUUID:         "4c4c4544-0042-3510-8051-b3c04f4e3332",
		},
	}

	deployment := testDeployment(startedAt)
	deployment.MediaURL = ""

	tokenUUID := deployment.Request.TokenUUID.String()

	exchanges := []api.BMCRecordingExchange{
		{
			Method:      "POST",
			URL:         "https://BMC.Corp.Example.Net/redfish/v1/Managers/1/VirtualMedia/CD/Actions/VirtualMedia.InsertMedia",
			RequestBody: `{"Image":"https://oc.corp.example.net:7443/1.0/provisioning/tokens/` + tokenUUID + `/seeds/seed01/image.iso"}`,
			Status:      204,
		},
		{
			Method:       "GET",
			URL:          "https://bmc.corp.example.net/redfish/v1/Managers/1",
			Status:       200,
			ResponseBody: `{"SerialNumber":"MGRSN98765","AssetTag":"R&D-07","Oem":{"Dell":{"SystemServiceTag":"7QZ1234"}},"SKU":"R640SKU77"}`,
		},
		{
			Method:       "GET",
			URL:          "https://bmc.corp.example.net/redfish/v1/Managers/1/EthernetInterfaces/1",
			Status:       200,
			ResponseBody: `{"HostName":"idrac-7QZ1234","PermanentMACAddress":"AABBCCDDEEFF","IPv6Addresses":[{"Address":"fe80::aabb:ccff:fedd:eeff"},{"Address":"2a02:1234::10"}],"Password":"Sup3rS3cret!"}`,
		},
		{
			Method:               "GET",
			URL:                  "https://bmc.corp.example.net/redfish/v1/Systems/1/Oem/Dump",
			Status:               200,
			ResponseBody:         base64.StdEncoding.EncodeToString(append([]byte("serial=SN123456 mgr=MGRSN98765 mac=aa:bb:cc:dd:ee:ff "), 0xff)),
			ResponseBodyEncoding: api.BMCRecordingBodyEncodingBase64,
		},
	}

	secrets := []string{
		"oc.corp.example.net",
		"bmc.corp.example.net",
		"r640-03",
		"bmcadmin",
		"Sup3rS3cret!",
		tokenUUID,
		"SN123456",
		"MGRSN98765",
		"R640SKU77",
		"R&D-07",
		`R&D-07`,
		"7QZ1234",
		"aa:bb:cc:dd:ee:ff",
		"aabbccddeeff",
		"fe80::aabb:ccff:fedd:eeff",
		"2a02:1234::10",
		"4c4c4544-0042-3510-8051-b3c04f4e3332",
	}

	tests := []struct {
		name     string
		finalize func(t *testing.T, session provisioning.BMCRecordingSession)
	}{
		{
			name:     "running deployment",
			finalize: func(t *testing.T, session provisioning.BMCRecordingSession) { t.Helper() },
		},
		{
			name: "recording closed without its deployment",
			finalize: func(t *testing.T, session provisioning.BMCRecordingSession) {
				t.Helper()

				require.NoError(t, session.Finalize(t.Context(), provisioning.ServerDeployment{}))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo, _ := newTestRepo(t)

			session, err := repo.Open(t.Context(), server, deployment)
			require.NoError(t, err)

			for _, exchange := range exchanges {
				session.Record(t.Context(), exchange)
			}

			tc.finalize(t, session)

			manifest, err := repo.GetByID(t.Context(), server.Name, recordingID)
			require.NoError(t, err)

			rewrite := provisioning.NewBMCRecordingRewrite(server, *manifest, operationsCenterAddress)

			// Run test
			rc, _, err := repo.GetArchive(t.Context(), server.Name, recordingID, rewrite)
			require.NoError(t, err)

			defer rc.Close()

			content, err := io.ReadAll(rc)
			require.NoError(t, err)

			// Assert
			text := anonymizedArchiveText(t, content)

			require.Contains(t, text, "operations-center.example", "the Operations Center host is replaced by its placeholder")

			for _, secret := range secrets {
				require.NotContains(t, strings.ToLower(text), strings.ToLower(secret), "the anonymized archive must not hold %q", secret)
			}
		})
	}
}

// anonymizedArchiveText returns the files of the archive as text. The bodies
// of the exchanges are added once more, base64 decoded, so a test can search
// them.
func anonymizedArchiveText(t *testing.T, content []byte) string {
	t.Helper()

	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)

	var text strings.Builder
	for _, f := range archive.File {
		r, err := f.Open()
		require.NoError(t, err)

		data, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())

		text.Write(data)

		if f.Name != "exchanges.jsonl" {
			continue
		}

		for line := range strings.Lines(string(data)) {
			var exchange api.BMCRecordingExchange
			require.NoError(t, json.Unmarshal([]byte(line), &exchange), "an anonymized exchange stays valid JSON")

			for _, body := range []struct{ data, encoding string }{
				{exchange.RequestBody, exchange.RequestBodyEncoding},
				{exchange.ResponseBody, exchange.ResponseBodyEncoding},
			} {
				decoded := []byte(body.data)
				if body.encoding == api.BMCRecordingBodyEncodingBase64 {
					decoded, err = base64.StdEncoding.DecodeString(body.data)
					require.NoError(t, err, "an anonymized base64 body stays valid base64")
				}

				text.Write(decoded)
			}
		}
	}

	return text.String()
}
