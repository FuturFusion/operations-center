package sort_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/sort"
)

func TestColumnsSorter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		columnSorters []sort.ColumnSorter

		want [][]string
	}{
		{
			name: "default - columns from 0 to 2, regular string sort",
			columnSorters: []sort.ColumnSorter{
				{
					Index:   0,
					Reverse: false,
					Less:    sort.StringLess,
				},
				{
					Index:   1,
					Reverse: false,
					Less:    sort.StringLess,
				},
				{
					Index:   2,
					Reverse: false,
					Less:    sort.StringLess,
				},
			},

			want: [][]string{
				{"a", "0", "bar"},
				{"a", "1", "foobar"},
				{"a", "11", "foobar"},
				{"a", "2", "foobar"},
				{"b", "", "baz"},
				{"b", "0", "baz"},
			},
		},
		{
			name: "default reverse - columns from 0 to 2, regular string sort all reverse",
			columnSorters: []sort.ColumnSorter{
				{
					Index:   0,
					Reverse: true,
					Less:    sort.StringLess,
				},
				{
					Index:   1,
					Reverse: true,
					Less:    sort.StringLess,
				},
				{
					Index:   2,
					Reverse: true,
					Less:    sort.StringLess,
				},
			},

			want: [][]string{
				{"b", "0", "baz"},
				{"b", "", "baz"},
				{"a", "2", "foobar"},
				{"a", "11", "foobar"},
				{"a", "1", "foobar"},
				{"a", "0", "bar"},
			},
		},
		{
			name: "reverse columns - columns 2 to 0, regular string sort",
			columnSorters: []sort.ColumnSorter{
				{
					Index:   2,
					Reverse: false,
					Less:    sort.StringLess,
				},
				{
					Index:   1,
					Reverse: false,
					Less:    sort.StringLess,
				},
				{
					Index:   0,
					Reverse: false,
					Less:    sort.StringLess,
				},
			},

			want: [][]string{
				{"a", "0", "bar"},
				{"b", "", "baz"},
				{"b", "0", "baz"},
				{"a", "1", "foobar"},
				{"a", "11", "foobar"},
				{"a", "2", "foobar"},
			},
		},
		{
			name: "natural sort - columns from 0 to 2, natural sort",
			columnSorters: []sort.ColumnSorter{
				{
					Index:   0,
					Reverse: false,
					Less:    sort.NaturalLess,
				},
				{
					Index:   1,
					Reverse: false,
					Less:    sort.NaturalLess,
				},
				{
					Index:   2,
					Reverse: false,
					Less:    sort.NaturalLess,
				},
			},

			want: [][]string{
				{"a", "0", "bar"},
				{"a", "1", "foobar"},
				{"a", "2", "foobar"},
				{"a", "11", "foobar"},
				{"b", "0", "baz"},
				{"b", "", "baz"},
			},
		},
		{
			name: "invalid column indexes are ignored",
			columnSorters: []sort.ColumnSorter{
				{
					Index:   -1,
					Reverse: false,
					Less:    sort.StringLess,
				},
				{
					Index:   3,
					Reverse: false,
					Less:    sort.StringLess,
				},
			},

			want: [][]string{
				{"a", "1", "foobar"},
				{"a", "2", "foobar"},
				{"a", "11", "foobar"},
				{"a", "0", "bar"},
				{"b", "0", "baz"},
				{"b", "", "baz"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := [][]string{
				{"a", "1", "foobar"},
				{"a", "2", "foobar"},
				{"a", "11", "foobar"},
				{"a", "0", "bar"},
				{"b", "0", "baz"},
				{"b", "", "baz"},
			}

			sort.ColumnsSort(data, tc.columnSorters)

			require.Equal(t, tc.want, data, "sorted columns")
		})
	}
}

func TestNaturalLess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		str1 string
		str2 string

		want bool
	}{
		{
			name: "natural order",
			str1: "server2",
			str2: "server10",

			want: true,
		},
		{
			name: "natural order reversed",
			str1: "server10",
			str2: "server2",

			want: false,
		},
		{
			name: "empty first value sorts last",
			str1: "",
			str2: "a",

			want: false,
		},
		{
			name: "empty second value sorts last",
			str1: "a",
			str2: "",

			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			got := sort.NaturalLess(tc.str1, tc.str2)

			// Assert
			require.Equal(t, tc.want, got, "%q less than %q", tc.str1, tc.str2)
		})
	}
}
