package dbschema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ensureDBRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		after           map[string]int
		decreaseAllowed map[string]struct{}

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:            "row count unchanged",
			after:           map[string]int{"one": 2, "two": 2},
			decreaseAllowed: nil,

			assertErr: require.NoError,
		},
		{
			name:            "row count increased",
			after:           map[string]int{"one": 3, "two": 2},
			decreaseAllowed: nil,

			assertErr: require.NoError,
		},
		{
			name:            "row count decreased in allowed table",
			after:           map[string]int{"one": 1, "two": 2},
			decreaseAllowed: map[string]struct{}{"one": {}},

			assertErr: require.NoError,
		},
		{
			name:            "error - row count decreased",
			after:           map[string]int{"one": 1, "two": 2},
			decreaseAllowed: nil,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `number of rows for table "one" decreased from 2 to 1`, a...)
			},
		},
		{
			name:            "error - row count decreased in other table than the allowed one",
			after:           map[string]int{"one": 2, "two": 1},
			decreaseAllowed: map[string]struct{}{"one": {}},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `number of rows for table "two" decreased from 2 to 1`, a...)
			},
		},
		{
			name:            "error - table missing after update",
			after:           map[string]int{"one": 2},
			decreaseAllowed: nil,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `no after counts for table "two"`, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			before := map[string]int{"one": 2, "two": 2}

			err := ensureDBRows(before, tc.after, tc.decreaseAllowed)

			tc.assertErr(t, err, "row count check")
		})
	}
}
