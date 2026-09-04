package seedprogress

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_coverage_add(t *testing.T) {
	type read struct {
		offset int64
		n      int64
	}

	tests := []struct {
		name  string
		reads []read

		wantBytes int64
		wantSpans []span
	}{
		{
			name:      "a read continuing where the last one stopped extends its span",
			reads:     []read{{0, 10}, {10, 10}, {20, 10}},
			wantBytes: 30,
			wantSpans: []span{{0, 30}},
		},
		{
			name:      "a range read twice is only counted once",
			reads:     []read{{0, 10}, {0, 10}},
			wantBytes: 10,
			wantSpans: []span{{0, 10}},
		},
		{
			name:      "overlapping reads are only counted once",
			reads:     []read{{0, 10}, {5, 10}},
			wantBytes: 15,
			wantSpans: []span{{0, 15}},
		},
		{
			name:      "a read fully inside a span adds nothing",
			reads:     []read{{0, 100}, {20, 10}},
			wantBytes: 100,
			wantSpans: []span{{0, 100}},
		},
		{
			name:      "disjoint reads are kept apart",
			reads:     []read{{0, 10}, {20, 10}},
			wantBytes: 20,
			wantSpans: []span{{0, 10}, {20, 30}},
		},
		{
			name:      "a read bridging two spans merges them",
			reads:     []read{{0, 10}, {20, 10}, {5, 20}},
			wantBytes: 30,
			wantSpans: []span{{0, 30}},
		},
		{
			name:      "a read before the spans recorded so far is inserted in front of them",
			reads:     []read{{20, 10}, {0, 10}},
			wantBytes: 20,
			wantSpans: []span{{0, 10}, {20, 30}},
		},
		{
			name:      "two spans, which only touch, are merged",
			reads:     []read{{20, 10}, {0, 10}, {10, 10}},
			wantBytes: 30,
			wantSpans: []span{{0, 30}},
		},
		{
			name:      "an empty read is ignored",
			reads:     []read{{10, 0}},
			wantBytes: 0,
			wantSpans: nil,
		},
		{
			name:      "a read at a negative offset is ignored",
			reads:     []read{{-1, 10}},
			wantBytes: 0,
			wantSpans: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c coverage

			for _, r := range tc.reads {
				c.add(r.offset, r.n)
			}

			require.Equal(t, tc.wantBytes, c.bytes(), "distinct bytes served")
			require.Equal(t, tc.wantSpans, c.spans, "recorded ranges")
		})
	}
}

func Test_coverage_matchesABitmap(t *testing.T) {
	const size = 1000

	var (
		c      coverage
		bitmap [size]bool
	)

	random := rand.New(rand.NewSource(1))

	for range 500 {
		offset := int64(random.Intn(size))
		n := min(int64(random.Intn(50)), size-offset)

		c.add(offset, n)

		for i := offset; i < offset+n; i++ {
			bitmap[i] = true
		}
	}

	want := int64(0)

	for _, covered := range bitmap {
		if covered {
			want++
		}
	}

	require.Equal(t, want, c.bytes(), "the distinct bytes served match a brute force account of them")
}

func Test_coverage_compaction(t *testing.T) {
	const (
		spans = maxCoverageSpans + 50
		read  = int64(10)
	)

	var c coverage

	// Every read is disjoint from the one before, so the record grows by one
	// span per read until it hits the limit. The gaps grow, so which pair is
	// coalesced is determined rather than arbitrary.
	offset := int64(0)
	for i := range spans {
		c.add(offset, read)
		offset += read + int64(i) + 1
	}

	require.LessOrEqual(t, len(c.spans), maxCoverageSpans, "the number of ranges stays bounded")
	require.GreaterOrEqual(t, c.bytes(), int64(spans)*read, "coalescing ranges never loses bytes")
	require.LessOrEqual(t, c.bytes(), c.spans[len(c.spans)-1].end-c.spans[0].start, "coverage stays within the range actually touched")
}
