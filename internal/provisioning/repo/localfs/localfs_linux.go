//go:build linux

package localfs

import (
	"context"
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func (l localfs) UsageInformation(_ context.Context) (provisioning.UsageInformation, error) {
	var stat unix.Statfs_t

	err := unix.Statfs(l.storageDir, &stat)
	if err != nil {
		return provisioning.UsageInformation{}, fmt.Errorf("Failed to statfs for %q: %w", l.storageDir, err)
	}

	// space in bytes = blocks * size per block
	return provisioning.UsageInformation{
		TotalSpaceBytes:     stat.Blocks * uint64(stat.Bsize),
		AvailableSpaceBytes: stat.Bavail * uint64(stat.Bsize),
		UsedSpaceBytes:      (stat.Blocks - stat.Bavail) * uint64(stat.Bsize),
	}, nil
}
