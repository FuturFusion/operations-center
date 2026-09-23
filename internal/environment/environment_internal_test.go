package environment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestEnvironment_GetSecureBootPlatformKeyUpdate(t *testing.T) {
	update := []byte("a signed platform key update")

	tests := []struct {
		name string

		isIncusOS bool
		content   []byte
		absent    bool

		want      []byte
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			isIncusOS: true,
			content:   update,

			want:      update,
			assertErr: require.NoError,
		},
		{
			name: "error - not an IncusOS system",

			isIncusOS: false,
			content:   update,

			assertErr: errassert.Contains("Not an IncusOS system"),
		},
		{
			name: "error - the update is not on the EFI system partition",

			isIncusOS: true,
			absent:    true,

			assertErr: errassert.Contains("Failed to read the IncusOS secure boot platform key update"),
		},
		{
			// An empty file enrolls nothing, so it is reported rather than being
			// built into a media, that leaves the server in setup mode.
			name: "error - the update is empty",

			isIncusOS: true,
			content:   []byte{},

			assertErr: errassert.Contains("is empty"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "PK.auth")

			if !tc.absent {
				require.NoError(t, os.WriteFile(path, tc.content, 0o600))
			}

			env := environment{
				platformKeyUpdatePath: path,
				isIncusOS:             func() bool { return tc.isIncusOS },
			}

			got, err := env.GetSecureBootPlatformKeyUpdate(t.Context())

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
