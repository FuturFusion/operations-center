package metrics

// retryable matches domain.ErrRetryable. The util layer must not import the
// domain package.
type retryable interface {
	error
	Retryable() bool
}

// IsRetryable reports whether err is retryable. A joined error is retryable
// only if all its errors are retryable. A hard failure then never counts as
// retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	r, ok := err.(retryable) //nolint:errorlint // The function unwraps err itself.
	if ok {
		return r.Retryable()
	}

	switch u := err.(type) { //nolint:errorlint // The function unwraps err itself.
	case interface{ Unwrap() []error }:
		return allRetryable(u.Unwrap())

	case interface{ Unwrap() error }:
		return IsRetryable(u.Unwrap())

	default:
		return false
	}
}

func allRetryable(errs []error) bool {
	found := false
	for _, err := range errs {
		if err == nil {
			continue
		}

		if !IsRetryable(err) {
			return false
		}

		found = true
	}

	return found
}
