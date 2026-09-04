package seedprogress

import (
	"slices"
	"sort"
)

const maxCoverageSpans = 1024

// span is a half open range [start, end) of an image, which has been served.
type span struct {
	start int64
	end   int64
}

func (s span) len() int64 {
	return s.end - s.start
}

// coverage is the set of the ranges of an image, which have been served. The
// ranges are kept sorted, disjoint and never touching, so that a range served
// more than once is only counted once.
type coverage struct {
	spans   []span
	covered int64
}

func (c *coverage) bytes() int64 {
	return c.covered
}

// add accounts for the n bytes at offset having been served.
func (c *coverage) add(offset int64, n int64) {
	if offset < 0 || n <= 0 {
		return
	}

	added := span{start: offset, end: offset + n}

	// A read continuing within or right after the range served last only
	// extends that range, which is what a reader streaming the image does.
	if len(c.spans) > 0 {
		last := &c.spans[len(c.spans)-1]
		if added.start >= last.start && added.start <= last.end {
			if added.end > last.end {
				c.covered += added.end - last.end
				last.end = added.end
			}

			return
		}
	}

	c.insert(added)
	c.compact()
}

// insert adds the range to the set, merging it with every range it overlaps or
// touches.
func (c *coverage) insert(added span) {
	// The first range, which can overlap or touch the one being added.
	// Comparing with >= rather than > merges two ranges, which only touch.
	i := sort.Search(len(c.spans), func(i int) bool {
		return c.spans[i].end >= added.start
	})

	if i == len(c.spans) || c.spans[i].start > added.end {
		c.spans = slices.Insert(c.spans, i, added)
		c.covered += added.len()

		return
	}

	merged := added

	j := i
	for j < len(c.spans) && c.spans[j].start <= merged.end {
		merged.start = min(merged.start, c.spans[j].start)
		merged.end = max(merged.end, c.spans[j].end)
		c.covered -= c.spans[j].len()
		j++
	}

	c.spans[i] = merged
	c.spans = slices.Delete(c.spans, i+1, j)
	c.covered += merged.len()
}

// compact keeps the number of ranges bounded by coalescing the two neighbors
// separated by the smallest gap.
func (c *coverage) compact() {
	for len(c.spans) > maxCoverageSpans {
		smallest := 0
		gap := c.spans[1].start - c.spans[0].end

		for i := 1; i < len(c.spans)-1; i++ {
			candidate := c.spans[i+1].start - c.spans[i].end
			if candidate < gap {
				smallest = i
				gap = candidate
			}
		}

		c.covered += gap
		c.spans[smallest].end = c.spans[smallest+1].end
		c.spans = slices.Delete(c.spans, smallest+1, smallest+2)
	}
}
