package cmds

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
)

func TestAdminBMCRecordingCommands(t *testing.T) {
	t.Parallel()

	// Setup
	admin := CmdAdmin{}

	// Run test
	cmd, _, err := admin.Command().Find([]string{"bmc-recording"})
	require.NoError(t, err)

	// Assert
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}

	require.Equal(t, "bmc-recording", cmd.Name(), "the group is registered below admin")
	require.ElementsMatch(t, []string{"list", "show", "download", "delete"}, names)
	require.Contains(t, cmd.Long, "internal and can change at any time")
}

func TestAdminBMCRecordingDownload(t *testing.T) {
	t.Parallel()

	noFile := func(*testing.T, string) {}

	writeFile := func(content string) func(t *testing.T, target string) {
		return func(t *testing.T, target string) {
			t.Helper()

			require.NoError(t, os.WriteFile(target, []byte(content), 0o600))
		}
	}

	zipHandler := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("zip content"))
	}

	fileContent := func(want string) func(t *testing.T, target string) {
		return func(t *testing.T, target string) {
			t.Helper()

			got, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, want, string(got), "an existing file is only replaced with --force")
		}
	}

	tests := []struct {
		name    string
		setup   func(t *testing.T, target string)
		handler http.HandlerFunc
		args    []string

		assertErr  require.ErrorAssertionFunc
		wantQuery  string
		assertFile func(t *testing.T, target string)
	}{
		{
			name:    "success - new file",
			setup:   noFile,
			handler: zipHandler,
			args:    []string{"one", "20261027T081500Z"},

			assertErr:  require.NoError,
			wantQuery:  "anonymize=false&archive=zip",
			assertFile: fileContent("zip content"),
		},
		{
			name:    "success - anonymized",
			setup:   noFile,
			handler: zipHandler,
			args:    []string{"one", "20261027T081500Z", "--anonymize"},

			assertErr:  require.NoError,
			wantQuery:  "anonymize=true&archive=zip",
			assertFile: fileContent("zip content"),
		},
		{
			name:    "error - existing file",
			setup:   writeFile("old"),
			handler: zipHandler,
			args:    []string{"one", "20261027T081500Z"},

			assertErr:  require.Error,
			wantQuery:  "anonymize=false&archive=zip",
			assertFile: fileContent("old"),
		},
		{
			name:    "success - existing file replaced with force",
			setup:   writeFile("old content that is longer"),
			handler: zipHandler,
			args:    []string{"one", "20261027T081500Z", "--force"},

			assertErr:  require.NoError,
			wantQuery:  "anonymize=false&archive=zip",
			assertFile: fileContent("zip content"),
		},
		{
			name:  "error - broken transfer removes the partial file",
			setup: noFile,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "100")
				_, _ = w.Write([]byte("zip con"))
			},
			args: []string{"one", "20261027T081500Z"},

			assertErr: require.Error,
			wantQuery: "anonymize=false&archive=zip",
			assertFile: func(t *testing.T, target string) {
				t.Helper()

				require.NoFileExists(t, target, "a partial archive is removed")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotQuery string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				tc.handler(w, r)
			}))
			t.Cleanup(server.Close)

			ocClient, err := client.New(server.URL)
			require.NoError(t, err)

			target := filepath.Join(t.TempDir(), "recording.zip")
			tc.setup(t, target)

			download := cmdAdminBMCRecordingDownload{ocClient: &ocClient}
			cmd := download.Command()
			cmd.Flags().Bool("quiet", true, "")
			cmd.SetArgs(append(tc.args, "--output", target))

			// Run test
			err = cmd.Execute()

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantQuery, gotQuery, "the query carries the anonymize flag")
			tc.assertFile(t, target)
		})
	}
}

func TestAdminBMCRecordingSubcommands(t *testing.T) {
	t.Parallel()

	const manifest = `{"server":"one","recording_id":"20261027T081500Z","started_at":"2026-10-27T08:15:00Z","final_state":"completed","exchanges":1250,"bytes":2048}`

	tests := []struct {
		name         string
		args         []string
		responseBody string

		assertErr  require.ErrorAssertionFunc
		wantMethod string
		wantPath   string
		wantOutput []string
	}{
		{
			name:         "success - list",
			args:         []string{"list", "one"},
			responseBody: `{"type":"sync","status":"Success","status_code":200,"metadata":[` + manifest + `]}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodGet,
			wantPath:   "/internal/bmc-recordings/one",
			wantOutput: []string{"20261027T081500Z", "completed", "1250", "2.05kB"},
		},
		{
			name:         "success - show as yaml",
			args:         []string{"show", "one", "20261027T081500Z"},
			responseBody: `{"type":"sync","status":"Success","status_code":200,"metadata":` + manifest + `}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodGet,
			wantPath:   "/internal/bmc-recordings/one/20261027T081500Z",
			wantOutput: []string{"recording_id: 20261027T081500Z", "exchanges: 1250"},
		},
		{
			name:         "success - show as json",
			args:         []string{"show", "one", "20261027T081500Z", "--format", "json"},
			responseBody: `{"type":"sync","status":"Success","status_code":200,"metadata":` + manifest + `}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodGet,
			wantPath:   "/internal/bmc-recordings/one/20261027T081500Z",
			wantOutput: []string{`"recording_id": "20261027T081500Z"`},
		},
		{
			name:         "error - show with invalid format",
			args:         []string{"show", "one", "20261027T081500Z", "--format", "table"},
			responseBody: `{}`,

			assertErr: require.Error,
		},
		{
			name:         "success - delete",
			args:         []string{"delete", "one", "20261027T081500Z"},
			responseBody: `{"type":"sync","status":"Success","status_code":200}`,

			assertErr:  require.NoError,
			wantMethod: http.MethodDelete,
			wantPath:   "/internal/bmc-recordings/one/20261027T081500Z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotMethod, gotPath string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path

				_, _ = w.Write([]byte(tc.responseBody))
			}))
			t.Cleanup(server.Close)

			ocClient, err := client.New(server.URL)
			require.NoError(t, err)

			recording := cmdAdminBMCRecording{ocClient: &ocClient}
			cmd := recording.Command()

			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs(tc.args)

			// Run test
			err = cmd.Execute()

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantMethod, gotMethod)
			require.Equal(t, tc.wantPath, gotPath)

			for _, want := range tc.wantOutput {
				require.Contains(t, output.String(), want)
			}
		})
	}
}

func TestAdminBMCRecordingDownloadDefaultFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		anonymize bool

		want string
	}{
		{
			name: "as recorded",

			want: "one-20261027T081500Z.zip",
		},
		{
			name:      "anonymized",
			anonymize: true,

			want: "one-20261027T081500Z-anonymized.zip",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			download := cmdAdminBMCRecordingDownload{flagAnonymize: tc.anonymize}

			// Run test
			got := download.targetFilename("one", "20261027T081500Z")

			// Assert
			require.Equal(t, tc.want, got, "a raw and an anonymized export do not collide")
		})
	}
}
