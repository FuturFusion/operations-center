package maps

import (
	"cmp"
	"iter"
	"slices"
)

// OrderedByKey returns an iterator to traverse a map ordered by key.
func OrderedByKey[K cmp.Ordered, E any](m map[K]E) iter.Seq2[K, E] {
	return func(yield func(K, E) bool) {
		keys := make([]K, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}

		slices.Sort(keys)

		for _, k := range keys {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}
