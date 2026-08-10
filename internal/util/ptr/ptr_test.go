package ptr_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func TestToInt64_int(t *testing.T) {
	tests := []struct {
		name string

		value *int

		want *int64
	}{
		{
			name:  "nil",
			value: nil,
			want:  nil,
		},
		{
			name:  "zero",
			value: ptr.To(0),
			want:  ptr.To(int64(0)),
		},
		{
			name:  "positive",
			value: ptr.To(20),
			want:  ptr.To(int64(20)),
		},
		{
			name:  "negative",
			value: ptr.To(-20),
			want:  ptr.To(int64(-20)),
		},
		{
			name:  "minimum",
			value: ptr.To(math.MinInt64),
			want:  ptr.To(int64(math.MinInt64)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ptr.ToInt64(tc.value))
		})
	}
}

func TestToInt64_uint64(t *testing.T) {
	tests := []struct {
		name string

		value *uint64

		want *int64
	}{
		{
			name:  "nil",
			value: nil,
			want:  nil,
		},
		{
			name:  "zero",
			value: ptr.To(uint64(0)),
			want:  ptr.To(int64(0)),
		},
		{
			name:  "positive",
			value: ptr.To(uint64(20)),
			want:  ptr.To(int64(20)),
		},
		{
			name:  "maximum representable as int64",
			value: ptr.To(uint64(math.MaxInt64)),
			want:  ptr.To(int64(math.MaxInt64)),
		},
		{
			name:  "too large for int64, capped instead of wrapped",
			value: ptr.To(uint64(math.MaxInt64) + 1),
			want:  ptr.To(int64(math.MaxInt64)),
		},
		{
			name:  "maximum uint64, capped instead of wrapped",
			value: ptr.To(uint64(math.MaxUint64)),
			want:  ptr.To(int64(math.MaxInt64)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ptr.ToInt64(tc.value))
		})
	}
}
