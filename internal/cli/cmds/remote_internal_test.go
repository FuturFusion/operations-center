package cmds

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/cli"
)

func TestCmdRemoteAdd_validateArgsAndFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid name",
			args: []string{"oc", "https://oc.example.com:8443"},

			assertErr: require.NoError,
		},
		{
			name: "empty name",
			args: []string{"", "https://oc.example.com:8443"},

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorContains(tt, err, "can not be empty")
			},
		},
		{
			name: "reserved name local",
			args: []string{"local", "https://oc.example.com:8443"},

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorContains(tt, err, "reserved name")
			},
		},
		{
			name: "parent directory",
			args: []string{"../x", "https://oc.example.com:8443"},

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorContains(tt, err, "is invalid")
			},
		},
		{
			name: "subdirectory",
			args: []string{"a/b", "https://oc.example.com:8443"},

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorContains(tt, err, "is invalid")
			},
		},
		{
			name: "backslash",
			args: []string{`a\b`, "https://oc.example.com:8443"},

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorContains(tt, err, "is invalid")
			},
		},
		{
			name: "dot",
			args: []string{".", "https://oc.example.com:8443"},

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorContains(tt, err, "is invalid")
			},
		},
		{
			name: "dot dot",
			args: []string{"..", "https://oc.example.com:8443"},

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorContains(tt, err, "is invalid")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			c := &cmdRemoteAdd{flagAuthType: string(config.AuthTypeTLS)}

			// Run test
			err := c.validateArgsAndFlags(&cobra.Command{}, tc.args)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
