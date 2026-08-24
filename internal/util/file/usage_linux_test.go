//go:build linux

package file_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

func TestUsageInformationForPath(t *testing.T) {
	tests := []struct {
		name string
		path string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",
			path: t.TempDir(),

			assertErr: require.NoError,
		},
		{
			name: "error - path does not exist",
			path: filepath.Join(t.TempDir(), "does-not-exist"),

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotUI, err := file.UsageInformationForPath(tc.path)

			tc.assertErr(t, err)
			if err != nil {
				require.Equal(t, file.UsageInformation{}, gotUI)
				return
			}

			require.Positive(t, gotUI.TotalSpaceBytes)
			require.Positive(t, gotUI.AvailableSpaceBytes)
			require.Positive(t, gotUI.UsedSpaceBytes)
			require.Equal(t, gotUI.TotalSpaceBytes, gotUI.AvailableSpaceBytes+gotUI.UsedSpaceBytes)
		})
	}
}
