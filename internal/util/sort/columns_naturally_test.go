package sort_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/sort"
)

func TestColumnsNaturally(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		columns [][]string

		want [][]string
	}{
		{
			name:    "empty",
			columns: [][]string{},

			want: [][]string{},
		},
		{
			name: "numbers in natural order",
			columns: [][]string{
				{"server10"},
				{"server2"},
				{"server1"},
			},

			want: [][]string{
				{"server1"},
				{"server2"},
				{"server10"},
			},
		},
		{
			name: "first differing column decides",
			columns: [][]string{
				{"b", "1"},
				{"a", "2"},
				{"a", "10"},
				{"a", "1"},
			},

			want: [][]string{
				{"a", "1"},
				{"a", "2"},
				{"a", "10"},
				{"b", "1"},
			},
		},
		{
			name: "empty cells sort last",
			columns: [][]string{
				{"b", "a"},
				{"", "a"},
				{"a", "z"},
				{"b", ""},
			},

			want: [][]string{
				{"a", "z"},
				{"b", "a"},
				{"b", ""},
				{"", "a"},
			},
		},
		{
			name: "equal rows",
			columns: [][]string{
				{"a", "1"},
				{"a", "1"},
			},

			want: [][]string{
				{"a", "1"},
				{"a", "1"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			sort.ColumnsNaturally(tc.columns)

			// Assert
			require.Equal(t, tc.want, tc.columns, "sorted columns")
		})
	}
}
