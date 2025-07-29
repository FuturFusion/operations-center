package sort

import "sort"

func ColumnsSort(columns [][]string, sorters []ColumnSorter) {
	sort.Sort(columnsSorter{
		columns: columns,
		sorters: sorters,
	})
}

type columnsSorter struct {
	columns [][]string
	sorters []ColumnSorter
}

type ColumnSorter struct {
	Index   int
	Reverse bool
	Less    func(str1 string, str2 string) bool
}

func (c columnsSorter) Len() int {
	return len(c.columns)
}

func (c columnsSorter) Swap(i, j int) {
	c.columns[i], c.columns[j] = c.columns[j], c.columns[i]
}

func (c columnsSorter) Less(i, j int) bool {
	for _, sorter := range c.sorters {
		if sorter.Index >= len(c.columns[i]) || sorter.Index < 0 {
			continue
		}

		if c.columns[i][sorter.Index] == c.columns[j][sorter.Index] {
			continue
		}

		return sorter.Less(c.columns[i][sorter.Index], c.columns[j][sorter.Index]) != sorter.Reverse
	}

	return false
}
