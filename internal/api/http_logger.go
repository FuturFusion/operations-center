package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"github.com/FuturFusion/operations-center/internal/logger"
)

type httpErrorLogger struct{}

// Match "bad certificate" errors, caused by the operations-center client server
// trust check, if self signed certificates are used.
// See: https://github.com/FuturFusion/operations-center/blob/34c91da0d638f7bea2f730416752fc248ba41e5d/internal/client/client.go#L233-L267
var badCertificateRe = regexp.MustCompile(`http: TLS handshake error from [^\s]+ remote error: tls: bad certificate`)

func (httpErrorLogger) Write(p []byte) (n int, err error) {
	// "bad certificate" errors from the client are expected if self signed
	// certificates are used due to the server trust check first trying with
	// trusted party (public CA) certificates only.
	if badCertificateRe.Match(p) {
		return len(p), nil
	}

	slog.ErrorContext(context.Background(), "daemon http server error logger", logger.Err(errors.New(string(bytes.TrimSpace(p)))))
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
