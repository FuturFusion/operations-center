//go:build linux

package flasher

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func (c *imageCache) usageInformation() (provisioning.UsageInformation, error) {
	err := os.MkdirAll(c.dir, 0o700)
	if err != nil {
		return provisioning.UsageInformation{}, fmt.Errorf("Failed to create seed image cache directory %q: %w", c.dir, err)
	}

	var stat unix.Statfs_t

	err = unix.Statfs(c.dir, &stat)
	if err != nil {
		return provisioning.UsageInformation{}, fmt.Errorf("Failed to statfs for %q: %w", c.dir, err)
	}

	// space in bytes = blocks * size per block
	return provisioning.UsageInformation{
		TotalSpaceBytes:     stat.Blocks * uint64(stat.Bsize),
		AvailableSpaceBytes: stat.Bavail * uint64(stat.Bsize),
		UsedSpaceBytes:      (stat.Blocks - stat.Bavail) * uint64(stat.Bsize),
	}, nil
}
