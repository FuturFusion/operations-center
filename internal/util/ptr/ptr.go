package ptr

import (
	"math"
)

// To returns a pointer to the given value.
func To[T any](v T) *T {
	return &v
}

func From[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}

	return *v
}

func ToInt64[T int | uint64](v *T) *int64 {
	if v == nil {
		return nil
	}

	value := int64(*v)
	if *v > 0 && value < 0 {
		value = math.MaxInt64
	}

	return &value
}
