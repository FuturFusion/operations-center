package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

type httpErrorLogger struct{}

// Match "bad certificate" errors, caused by the operations-center client server
// trust check, if self signed certificates are used.
// Match "unknown certificate" errors, cause by operations-center clients not
// providing a client certificate, since authentication is using OIDC.
// See: https://github.com/FuturFusion/operations-center/blob/34c91da0d638f7bea2f730416752fc248ba41e5d/internal/client/client.go#L233-L267
var badCertificateRe = regexp.MustCompile(`http: TLS handshake error from [^\s]+ remote error: tls: (bad|unknown) certificate`)

func (httpErrorLogger) Write(p []byte) (n int, err error) {
	// "bad certificate" errors from the client are expected if self signed
	// certificates are used due to the server trust check first trying with
	// trusted party (public CA) certificates only.
	// "unknown certificate" errors are expected, if the client does not provide
	// a client certificate, since authentication is using OIDC.
	if badCertificateRe.Match(p) {
		slog.DebugContext(context.Background(), "expected daemon http server error", logger.Err(errors.New(string(bytes.TrimSpace(p)))))
		return len(p), nil
	}

	// Ignore TLS handshake errors from heartbeat / monitoring connection attempts by trusted https proxies.
	for _, proxyIP := range config.GetSecurity().TrustedHTTPSProxies {
		if bytes.Contains(p, []byte(proxyIP)) && bytes.Contains(p, []byte("http: TLS handshake error from")) {
			slog.DebugContext(context.Background(), "expected daemon http server error", logger.Err(errors.New(string(bytes.TrimSpace(p)))))
			return len(p), nil
		}
	}

	slog.ErrorContext(context.Background(), "daemon http server error logger", logger.Err(errors.New(string(bytes.TrimSpace(p)))))
	return len(p), nil
}

// shutdownGracePeriod is the time a background task gets to stop after the
// deadline of the shutdown has passed. An idle task stops within this time. A
// busy task does not extend the shutdown beyond its deadline.
const shutdownGracePeriod = 100 * time.Millisecond

// deadlineFrom returns the time until the deadline of ctx. Without a deadline,
// it returns defaultDeadline. After the deadline, it returns
// shutdownGracePeriod.
func deadlineFrom(ctx context.Context, defaultDeadline time.Duration) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return defaultDeadline
	}

	return max(time.Until(deadline), shutdownGracePeriod)
}
