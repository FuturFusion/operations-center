package provisioning

import (
	"fmt"
	"io"

	cli "github.com/lxc/incus/v6/shared/cmd"
	"github.com/lxc/incus/v6/shared/ioprogress"
	"github.com/lxc/incus/v6/shared/units"
)

func progressWriter(target io.WriteCloser, format string, quiet bool) (*cli.ProgressRenderer, io.WriteCloser) {
	progress := &cli.ProgressRenderer{
		Format: format,
		Quiet:  quiet,
	}

	tracker := &ioprogress.ProgressTracker{
		Handler: func(bytesReceived int64, speed int64) {
			progress.UpdateProgress(ioprogress.ProgressData{
				Text: fmt.Sprintf("%s (%s/s)",
					units.GetByteSizeString(bytesReceived, 2),
					units.GetByteSizeString(speed, 2)),
			})
		},
	}

	writer := &ioprogress.ProgressWriter{
		WriteCloser: target,
		Tracker:     tracker,
	}

	return progress, writer
}
