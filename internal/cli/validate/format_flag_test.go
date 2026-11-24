package validate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestFormatFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:  "csv",
			value: "csv",

			assertErr: require.NoError,
		},
		{
			name:  "json",
			value: "json",

			assertErr: require.NoError,
		},
		{
			name:  "table",
			value: "table",

			assertErr: require.NoError,
		},
		{
			name:  "yaml",
			value: "yaml",

			assertErr: require.NoError,
		},
		{
			name:  "compact",
			value: "compact",

			assertErr: require.NoError,
		},
		{
			name:  "table with header option",
			value: "table,header",

			assertErr: require.NoError,
		},
		{
			name:  "csv with noheader option",
			value: "csv,noheader",

			assertErr: require.NoError,
		},
		{
			name:  "error - unknown format",
			value: "xml",

			assertErr: errassert.Contains(`Invalid value for flag "--format": "xml"`),
		},
		{
			name:  "error - unknown format with valid option",
			value: "xml,header",

			assertErr: errassert.Contains(`Invalid value for flag "--format": "xml"`),
		},
		{
			name:  "error - unknown option",
			value: "table,unknown",

			assertErr: errassert.Contains(`Invalid option for flag "--format": "unknown"`),
		},
		{
			name:  "error - empty option",
			value: "table,",

			assertErr: errassert.Contains(`Invalid option for flag "--format": ""`),
		},
		{
			name:  "error - empty",
			value: "",

			assertErr: errassert.Contains(`Invalid value for flag "--format": ""`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			err := validate.FormatFlag(tc.value)

			// Assert
			tc.assertErr(t, err, "unexpected result for %q", tc.value)
		})
	}
}
