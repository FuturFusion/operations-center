//go:build linux

package flasher

import (
	"fmt"
	"os"

	"github.com/FuturFusion/operations-center/internal/util/file"
)

func (c *imageCache) usageInformation() (file.UsageInformation, error) {
	err := os.MkdirAll(c.dir, 0o700)
	if err != nil {
		return file.UsageInformation{}, fmt.Errorf("Failed to create seed image cache directory %q: %w", c.dir, err)
	}

	return file.UsageInformationForPath(c.dir)
}
