package api

import (
	"context"
	"log/slog"
	"time"
)

type httpErrorLogger struct{}

func (httpErrorLogger) Write(p []byte) (n int, err error) {
	slog.ErrorContext(context.Background(), string(p)) //nolint:sloglint // error message coming from the http server is the message.
	return len(p), nil
}

// deadlineFrom extracts the deadline from the provided context if present and not yet expired.
// Otherwise the defaultDeadline is returned.
func deadlineFrom(ctx context.Context, defaultDeadline time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if ok {
		deadlineDuration := time.Until(deadline)
		if deadlineDuration > 0 {
			return deadlineDuration
		}
	}

	return defaultDeadline
}
