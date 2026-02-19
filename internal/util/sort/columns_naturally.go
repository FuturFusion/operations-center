package sort

import (
	"sort"
)

// columnsNaturally represents the type for sorting columns in a natural order from left to right.
type columnsNaturally [][]string

func ColumnsNaturally(columns [][]string) {
	sort.Sort(columnsNaturally(columns))
}

func (c columnsNaturally) Len() int {
	return len(c)
}

func (c columnsNaturally) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c columnsNaturally) Less(i, j int) bool {
	for k := range c[i] {
		if c[i][k] == c[j][k] {
			continue
		}

		if c[i][k] == "" {
			return false
		}

		if c[j][k] == "" {
			return true
		}

		return NaturalLess(c[i][k], c[j][k])
	}

	return false
}
