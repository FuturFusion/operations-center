package dbschema

import (
	"testing"

	"github.com/stretchr/testify/require"

	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func TestQuoteString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string

		want string
	}{
		{
			name:  "plain",
			input: "plain",

			want: `'plain'`,
		},
		{
			name:  "single quote",
			input: "it's",

			want: `'it''s'`,
		},
		{
			name:  "newline",
			input: "a\nb",

			want: `replace('a\nb','\n',char(10))`,
		},
		{
			name:  "carriage return",
			input: "a\rb",

			want: `replace('a\rb','\r',char(13))`,
		},
		{
			name:  "newline and carriage return",
			input: "a\r\nb",

			want: `replace(replace('a\r\nb','\n',char(10)),'\r',char(13))`,
		},
		{
			name:  "newline and literal newline marker",
			input: `a\n` + "\n",

			want: `replace('a\n\012','\012',char(10))`,
		},
		{
			name:  "newline and both literal newline markers",
			input: `a\n\012` + "\n",

			want: `replace('a\n\012(\n0)','(\n0)',char(10))`,
		},
		{
			name:  "carriage return and literal carriage return marker",
			input: `a\r` + "\r",

			want: `replace('a\r\015','\015',char(13))`,
		},
		{
			name:  "carriage return and all literal carriage return markers",
			input: `a\r\015(\r0)` + "\r",

			want: `replace('a\r\015(\r0)(\r1)','(\r1)',char(13))`,
		},
		{
			name:  "nul byte",
			input: "a\x00b",

			want: `CAST(X'610062' AS TEXT)`,
		},
		{
			name:  "nul byte with single quote and newline",
			input: "a\x00'\n",

			want: `CAST(X'6100270A' AS TEXT)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			db, err := dbdriver.Open(t.TempDir())
			require.NoError(t, err, "opening the database should succeed")

			t.Cleanup(func() {
				err := db.Close()
				require.NoError(t, err, "closing the database should succeed")
			})

			// Run test
			got := quoteString(tc.input)

			// Assert
			require.Equal(t, tc.want, got, "literal should match the expected SQL")

			var restored string
			err = db.QueryRowContext(t.Context(), "SELECT "+got).Scan(&restored)
			require.NoError(t, err, "SQLite should accept the literal")
			require.Equal(t, tc.input, restored, "SQLite should evaluate the literal to the input")
		})
	}
}
