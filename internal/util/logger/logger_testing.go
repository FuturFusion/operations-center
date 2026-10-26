package logger

import (
	"context"
	"io"
	"testing"
)

// ContextWithTestLogger returns a context for the test tb with its own log
// destination. Log records emitted with this context are written to writer
// without color. They do not reach the destination configured with
// InitLogger. The arguments verbose and debug have the same meaning as for
// InitLogger. SetLogLevel and the log levels configured for components do not
// apply.
//
// This allows a test to capture its own log, even if it runs in parallel with
// other tests. DetachedContext keeps the log destination. Only records of the
// default slog logger reach the destination.
func ContextWithTestLogger(tb testing.TB, writer io.Writer, verbose bool, debug bool) context.Context {
	tb.Helper()

	ctx, err := contextWithLogger(tb.Context(), writer, verbose, debug)
	if err != nil {
		tb.Fatalf("Failed to install the default logger: %v", err)
	}

	return ctx
}
