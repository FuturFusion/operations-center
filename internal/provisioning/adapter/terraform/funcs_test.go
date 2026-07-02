package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_tfString(t *testing.T) {
	tests := []struct {
		name string
		in   string

		want string
	}{
		{
			name: "plain",
			in:   "value",

			want: "value",
		},
		{
			name: "multi-line",
			in:   "KEY1=value1\nKEY2=value2",

			want: `KEY1=value1\nKEY2=value2`,
		},
		{
			name: "quotes and backslashes",
			in:   `a "quoted" \value\`,

			want: `a \"quoted\" \\value\\`,
		},
		{
			name: "template sequences",
			in:   "${var.foo} %{if true}",

			want: "$${var.foo} %%{if true}",
		},
		{
			name: "carriage return and tab",
			in:   "a\r\nb\tc",

			want: `a\r\nb\tc`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tfString(tc.in)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_maxKeyLength(t *testing.T) {
	type customMap map[string]string

	tests := []struct {
		name string
		in   any

		want int
	}{
		{
			name: "not a map",
			in:   bool(false),

			want: 0,
		},
		{
			name: "map[string]any",
			in: map[string]any{
				"key":                false,
				"other_key":          false,
				"very_long_long_key": false,
			},

			want: 18,
		},
		{
			name: "map[string]string",
			in: map[string]string{
				"key":                "value 1",
				"other_key":          "value 2",
				"very_long_long_key": "value 3",
			},

			want: 18,
		},
		{
			name: "custom type",
			in: customMap{
				"key":                "value 1",
				"other_key":          "value 2",
				"very_long_long_key": "value 3",
			},

			want: 18,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maxKeyLength(tc.in)

			require.Equal(t, tc.want, got)
		})
	}
}
