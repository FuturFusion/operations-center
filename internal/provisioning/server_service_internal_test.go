package provisioning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_availableVersionGreaterThan(t *testing.T) {
	tests := []struct {
		name             string
		currentVersion   string
		availableVersion string

		want bool
	}{
		{
			name:             "available version greater",
			currentVersion:   "202601172317",
			availableVersion: "202601210123",

			want: true,
		},
		{
			name:             "available version equal",
			currentVersion:   "202601210123",
			availableVersion: "202601210123",

			want: false,
		},
		{
			name:             "available version smaller",
			currentVersion:   "202601210123",
			availableVersion: "202601172317",

			want: false,
		},
		{
			name:             "current invalid",
			currentVersion:   "invalid",
			availableVersion: "202601210123",

			want: true,
		},
		{
			name:             "available invalid",
			currentVersion:   "202601210123",
			availableVersion: "invalid",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := availableVersionGreaterThan(tc.currentVersion, tc.availableVersion)

			require.Equal(t, tc.want, got)
		})
	}
}
