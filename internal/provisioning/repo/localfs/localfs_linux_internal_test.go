//go:build linux

package localfs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalfs_UsageInformation(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "success",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			// tc.setupTmpDir(t, tmpDir)
			lfs, err := New(tmpDir, "")
			require.NoError(t, err)

			// Run test
			gotUI, err := lfs.UsageInformation(context.Background())

			// Assert
			require.NoError(t, err)
			require.Positive(t, gotUI.TotalSpaceBytes)
			require.Positive(t, gotUI.AvailableSpaceBytes)
			require.Positive(t, gotUI.UsedSpaceBytes)
			require.Equal(t, gotUI.TotalSpaceBytes, gotUI.AvailableSpaceBytes+gotUI.UsedSpaceBytes)
			t.Log(gotUI)
		})
	}
}
