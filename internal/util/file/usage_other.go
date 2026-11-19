//go:build !linux

package file

import (
	"errors"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// UsageInformationForPath returns the space usage of the file system, the given
// path is located on. It is only supported on Linux and returns an error
// wrapping [errors.ErrUnsupported] on all other operating systems.
func UsageInformationForPath(name string) (domain.UsageInformation, error) {
	return domain.UsageInformation{}, fmt.Errorf("Failed to get usage information for %q: %w", name, errors.ErrUnsupported)
}
