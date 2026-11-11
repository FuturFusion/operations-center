package file_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

func TestCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, source string, dest string)

		assertErr  require.ErrorAssertionFunc
		wantTarget string
		wantData   string
	}{
		{
			name: "success - regular file",
			setup: func(t *testing.T, source string, _ string) {
				t.Helper()
				require.NoError(t, os.WriteFile(source, []byte("content"), 0o600))
			},

			assertErr:  require.NoError,
			wantTarget: "",
			wantData:   "content",
		},
		{
			name: "success - regular file overwrites existing destination",
			setup: func(t *testing.T, source string, dest string) {
				t.Helper()
				require.NoError(t, os.WriteFile(source, []byte("content"), 0o600))
				require.NoError(t, os.WriteFile(dest, []byte("old and longer content"), 0o600))
			},

			assertErr:  require.NoError,
			wantTarget: "",
			wantData:   "content",
		},
		{
			name: "success - symlink replaces existing destination",
			setup: func(t *testing.T, source string, dest string) {
				t.Helper()
				require.NoError(t, os.Symlink("target", source))
				require.NoError(t, os.WriteFile(dest, []byte("old content"), 0o600))
			},

			assertErr:  require.NoError,
			wantTarget: "target",
			wantData:   "",
		},
		{
			name:  "error - source does not exist",
			setup: func(t *testing.T, _ string, _ string) { t.Helper() },

			assertErr:  require.Error,
			wantTarget: "",
			wantData:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			dest := filepath.Join(dir, "dest")
			tc.setup(t, source, dest)

			// Run test
			err := file.Copy(source, dest)

			// Assert
			tc.assertErr(t, err)

			gotTarget, _ := os.Readlink(dest)
			require.Equal(t, tc.wantTarget, gotTarget, "symlink target of the destination")

			gotData, _ := os.ReadFile(dest)
			require.Equal(t, tc.wantData, string(gotData), "content of the destination")
		})
	}
}
