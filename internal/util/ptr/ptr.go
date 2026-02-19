package ptr

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
