package provisioning

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
)

func TestCmdUpdateFileGet_run_existingTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing string

		wantContent string
	}{
		{
			name:     "keeps content of existing target",
			existing: "existing content",

			wantContent: "existing content",
		},
		{
			name:     "keeps empty existing target",
			existing: "",

			wantContent: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"type":"sync","status":"Success","status_code":200,"metadata":[{"filename":"file.bin"}]}`))
			}))
			t.Cleanup(srv.Close)

			ocClient, err := client.New(srv.URL)
			require.NoError(t, err)

			target := filepath.Join(t.TempDir(), "target")
			require.NoError(t, os.WriteFile(target, []byte(tc.existing), 0o600))

			c := &cmdUpdateFileGet{ocClient: &ocClient}
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())

			// Run test
			err = c.run(cmd, []string{"id", "file.bin", target})

			// Assert
			require.ErrorIs(t, err, os.ErrExist)

			got, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, tc.wantContent, string(got), "existing target must not be modified or removed")
		})
	}
}
