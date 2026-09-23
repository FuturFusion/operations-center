// Package service stands in for a package holding business logic. It is
// recognised as one because of the _service.go file, which is how the scan
// tells business logic from the rest without a list anybody has to maintain.
package service

import (
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// UnkindedError originates here and carries no kind, so nothing says, whether
// it is meant for the user.
func UnkindedError(name string) error {
	return fmt.Errorf("The cluster %q is too small", name)
}

// InternalError says, that it is internal by design, so it is not reported.
func InternalError(state string) error {
	//domain-errors:internal Programmer error, the state table and the state disagree.
	return fmt.Errorf("State %q is not an action", state)
}

// WrapKeepsTheKindOfWhatItWraps adds context for the log only.
func WrapKeepsTheKindOfWhatItWraps(name string, err error) error {
	return fmt.Errorf("Failed to get cluster %q: %w", name, err)
}

// RetryableIsAClassification hands the error to a constructor of the domain
// package, which classifies it.
func RetryableIsAClassification(name string) error {
	return domain.NewRetryableErr(fmt.Errorf("Cluster %q is busy", name))
}

// ValidationIsAClassification is the short form for the validation of input.
func ValidationIsAClassification(name string) error {
	return domain.NewValidationErrf("Cluster name %q is not valid", name)
}
