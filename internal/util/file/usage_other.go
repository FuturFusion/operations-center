//go:build !linux

package file

import (
	"errors"
	"fmt"
)

// UsageInformationForPath returns the space usage of the file system, the given
// path is located on. It is only supported on Linux and returns an error
// wrapping [errors.ErrUnsupported] on all other operating systems.
func UsageInformationForPath(name string) (UsageInformation, error) {
	return UsageInformation{}, fmt.Errorf("Failed to get usage information for %q: %w", name, errors.ErrUnsupported)
}
