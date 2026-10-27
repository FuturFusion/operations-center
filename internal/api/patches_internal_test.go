package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/fsroot"
)

func Test_patchRemoveVarLibOperationCenterTerraformDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)

		wantEntries []string
	}{
		{
			name: "success - directories exist",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				for _, name := range []string{"servercerts", "terraform/cluster-configs/cluster", "updates"} {
					err := os.MkdirAll(filepath.Join(dir, name), 0o700)
					require.NoError(t, err)
				}

				err := os.WriteFile(filepath.Join(dir, "servercerts", "cluster.crt"), nil, 0o600)
				require.NoError(t, err)
			},

			wantEntries: []string{"updates"},
		},
		{
			name: "success - directories do not exist",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				err := os.Mkdir(filepath.Join(dir, "updates"), 0o700)
				require.NoError(t, err)
			},

			wantEntries: []string{"updates"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()
			tc.setup(t, tmpDir)

			d := &Daemon{
				varRoot: fsroot.Open(t, tmpDir),
			}

			// Run test
			err := patchRemoveVarLibOperationCenterTerraformDirs(t.Context(), "test", d)

			// Assert
			require.NoError(t, err)

			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)
			gotEntries := make([]string, 0, len(entries))
			for _, entry := range entries {
				gotEntries = append(gotEntries, entry.Name())
			}

			require.Equal(t, tc.wantEntries, gotEntries, "only the servercerts and terraform directories are removed from the var directory")
		})
	}
}
