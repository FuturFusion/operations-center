package provisioning

import (
	"context"
	"io"
)

// SeedImageProgressPort records how much of a seed image the sources reading it
// have read.
type SeedImageProgressPort interface {
	// Track wraps content, so that the reads served from it are recorded as
	// progress of source reading the image identified by imageID.
	Track(ctx context.Context, imageID SeedImageID, source string, size int64, content io.ReadSeekCloser) io.ReadSeekCloser

	// Get returns the progress recorded for imageID being read by source and
	// reports whether anything has been recorded at all.
	Get(ctx context.Context, imageID SeedImageID, source string) (SeedImageProgress, bool)
}
