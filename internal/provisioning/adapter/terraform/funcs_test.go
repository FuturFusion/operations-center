package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
