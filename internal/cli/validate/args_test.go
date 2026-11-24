package validate_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		minArgs int
		maxArgs int

		wantExit        bool
		assertErr       require.ErrorAssertionFunc
		assertHelp      require.ValueAssertionFunc
		wantOriginalRun bool
	}{
		{
			name:    "within range",
			args:    []string{"a"},
			minArgs: 1,
			maxArgs: 2,

			wantExit:        false,
			assertErr:       require.NoError,
			assertHelp:      require.Empty,
			wantOriginalRun: true,
		},
		{
			name:    "no upper limit",
			args:    []string{"a", "b", "c", "d"},
			minArgs: 1,
			maxArgs: -1,

			wantExit:        false,
			assertErr:       require.NoError,
			assertHelp:      require.Empty,
			wantOriginalRun: true,
		},
		{
			name:    "no args shows help without error",
			args:    []string{},
			minArgs: 1,
			maxArgs: 1,

			wantExit:        true,
			assertErr:       require.NoError,
			assertHelp:      require.NotEmpty,
			wantOriginalRun: false,
		},
		{
			name:    "error - too few args",
			args:    []string{"a"},
			minArgs: 2,
			maxArgs: 2,

			wantExit:        true,
			assertErr:       errassert.Contains("Invalid number of arguments"),
			assertHelp:      require.NotEmpty,
			wantOriginalRun: false,
		},
		{
			name:    "error - too many args",
			args:    []string{"a", "b"},
			minArgs: 0,
			maxArgs: 1,

			wantExit:        true,
			assertErr:       errassert.Contains("Invalid number of arguments"),
			assertHelp:      require.NotEmpty,
			wantOriginalRun: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			originalRun := false
			cmd := &cobra.Command{
				Use: "test",
				Run: func(*cobra.Command, []string) {
					originalRun = true
				},
			}

			help := &bytes.Buffer{}
			cmd.SetOut(help)

			// Run test
			exit, err := validate.Args(cmd, tc.args, tc.minArgs, tc.maxArgs)
			cmd.Run(cmd, tc.args)

			// Assert
			require.Equal(t, tc.wantExit, exit, "exit requested")
			tc.assertErr(t, err, "error of Args")
			tc.assertHelp(t, help.String(), "help output")
			require.Equal(t, tc.wantOriginalRun, originalRun, "original run function called")
		})
	}
}

func TestArgs_exitReplacesRunE(t *testing.T) {
	t.Parallel()

	// Setup
	cmd := &cobra.Command{
		Use:  "test",
		RunE: func(*cobra.Command, []string) error { return boom.Error },
	}

	cmd.SetOut(&bytes.Buffer{})

	// Run test
	exit, _ := validate.Args(cmd, []string{}, 1, 1)
	err := cmd.RunE(cmd, nil)

	// Assert
	require.True(t, exit, "exit requested")
	require.NoError(t, err, "RunE is replaced with a function that does nothing")
}
