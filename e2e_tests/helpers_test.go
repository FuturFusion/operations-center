package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_cmd(t *testing.T) {
	tests := []struct {
		name string
		cmd  string

		wantSuccess bool
		wantOutput  string
	}{
		{
			name: "true",
			cmd:  "true",

			wantSuccess: true,
		},
		{
			name: "false",
			cmd:  "false",

			wantSuccess: false,
		},
		{
			name: "echo with arguments",
			cmd:  "echo -n foo bar baz",

			wantSuccess: true,
			wantOutput:  "foo bar baz",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := run(t, `%s`, tc.cmd)
			require.NoError(t, err)

			require.Equal(t, tc.wantSuccess, resp.Success())
			require.Equal(t, tc.wantOutput, resp.Output())
		})
	}
}
