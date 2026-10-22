package logger

import (
	"context"
	"io"
	"testing"
)

// ContextWithTestLogger returns a context for the test tb with its own log
// destination. Log records emitted with this context are written to writer.
// They do not reach the destination configured with InitLogger. The arguments
// verbose, debug and noColor have the same meaning as for InitLogger. Log
// levels configured for components do not apply.
//
// This allows a test to capture its own log, also if it runs in parallel with
// other tests. DetachedContext keeps the log destination.
func ContextWithTestLogger(tb testing.TB, writer io.Writer, verbose bool, debug bool, noColor bool) context.Context {
	tb.Helper()

	return contextWithLogger(tb.Context(), writer, verbose, debug, noColor)
}
