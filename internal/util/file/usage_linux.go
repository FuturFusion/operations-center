//go:build linux

package file

import (
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// UsageInformationForPath returns the space usage of the file system, the given
// path is located on.
func UsageInformationForPath(name string) (domain.UsageInformation, error) {
	var stat unix.Statfs_t

	err := unix.Statfs(name, &stat)
	if err != nil {
		return domain.UsageInformation{}, fmt.Errorf("Failed to statfs for %q: %w", name, err)
	}

	// space in bytes = blocks * size per block
	return domain.UsageInformation{
		TotalSpaceBytes:     stat.Blocks * uint64(stat.Bsize),
		AvailableSpaceBytes: stat.Bavail * uint64(stat.Bsize),
		UsedSpaceBytes:      (stat.Blocks - stat.Bavail) * uint64(stat.Bsize),
	}, nil
}
