// Package sqlite stands in for the infrastructure layer, which may hand a bare
// kind to its caller. It sits under a repo directory on purpose, that is how
// the layer is recognised.
package sqlite

import (
	"github.com/FuturFusion/operations-center/internal/domain"
)

// SentinelIsAllowedHere hands the kind to the service above, which knows the
// name to put into the message and turns it into an error for the user.
func SentinelIsAllowedHere(found bool) error {
	if !found {
		return domain.ErrNotFound
	}

	return nil
}
