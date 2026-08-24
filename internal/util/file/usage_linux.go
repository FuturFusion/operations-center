//go:build linux

package file

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// UsageInformationForPath returns the space usage of the file system, the given
// path is located on.
func UsageInformationForPath(name string) (UsageInformation, error) {
	var stat unix.Statfs_t

	err := unix.Statfs(name, &stat)
	if err != nil {
		return UsageInformation{}, fmt.Errorf("Failed to statfs for %q: %w", name, err)
	}

	// space in bytes = blocks * size per block
	return UsageInformation{
		TotalSpaceBytes:     stat.Blocks * uint64(stat.Bsize),
		AvailableSpaceBytes: stat.Bavail * uint64(stat.Bsize),
		UsedSpaceBytes:      (stat.Blocks - stat.Bavail) * uint64(stat.Bsize),
	}, nil
}
