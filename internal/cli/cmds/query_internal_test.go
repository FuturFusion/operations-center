package cmds

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
)

func TestCmdQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string

		wantMethod      string
		wantPath        string
		wantContentType string
		wantBody        string
	}{
		{
			name: "GET without data",
			args: []string{"/1.0/system/settings"},

			wantMethod:      http.MethodGet,
			wantPath:        "/1.0/system/settings",
			wantContentType: "application/json",
			wantBody:        "",
		},
		{
			name: "PUT with data",
			args: []string{"-X", http.MethodPut, "-d", `{"log_level":"DEBUG"}`, "/1.0/system/settings"},

			wantMethod:      http.MethodPut,
			wantPath:        "/1.0/system/settings",
			wantContentType: "application/json",
			wantBody:        `{"log_level":"DEBUG"}`,
		},
		{
			name: "POST to internal API",
			args: []string{"-X", http.MethodPost, "-d", `{"query":"SELECT 1"}`, "/internal/sql"},

			wantMethod:      http.MethodPost,
			wantPath:        "/internal/sql",
			wantContentType: "application/json",
			wantBody:        `{"query":"SELECT 1"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			var gotMethod, gotPath, gotContentType, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)

				gotMethod = r.Method
				gotPath = r.URL.Path
				gotContentType = r.Header.Get("Content-Type")
				gotBody = string(body)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"type":"sync","status":"Success","status_code":200,"metadata":{}}`))
			}))
			t.Cleanup(srv.Close)

			ocClient, err := client.New(srv.URL)
			require.NoError(t, err)

			cmd := (&CmdQuery{OCClient: &ocClient}).Command()
			cmd.SetArgs(tc.args)
			cmd.SetOut(&bytes.Buffer{})

			// Run test
			err = cmd.Execute()

			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.wantMethod, gotMethod)
			require.Equal(t, tc.wantPath, gotPath)
			require.Equal(t, tc.wantContentType, gotContentType, "the API requires application/json on JSON endpoints")
			require.Equal(t, tc.wantBody, gotBody)
		})
	}
}
