package logger

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetLevelsKeepsLevelsOnHandlerFailure(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, InitLogger(logBuf, "", false, false, true), "logger initialization must not fail")

	t.Cleanup(func() {
		require.NoError(t, InitLogger(logBuf, "", false, false, true), "the component levels must not leak into other tests")
	})

	require.NoError(t, SetComponentLevels(map[string]slog.Level{"api": LevelTrace}), "setting the component levels must not fail")

	inEffect := levels.Load()

	// A log file below a directory which does not exist can not be opened, so
	// the installation of the handler fails.
	err := InitLogger(logBuf, filepath.Join(t.TempDir(), "missing", "operations-center.log"), false, false, true)

	require.Error(t, err, "initialization fails if the log file can not be opened")
	require.Same(t, inEffect, levels.Load(), "a failed handler installation leaves the levels in effect untouched")
}

func TestSetLevelsRebuildsHandlerOnlyOnRenderingChange(t *testing.T) {
	logBuf := &bytes.Buffer{}

	require.NoError(t, InitLogger(logBuf, "", false, false, true), "logger initialization must not fail")

	t.Cleanup(func() {
		require.NoError(t, InitLogger(logBuf, "", false, false, true), "the component levels must not leak into other tests")
	})

	handler := slog.Default().Handler()

	require.NoError(t, SetComponentLevels(map[string]slog.Level{"api": slog.LevelError}), "setting the component levels must not fail")
	require.Same(t, handler, slog.Default().Handler(), "an override less verbose than the default does not change how records are rendered")

	require.NoError(t, SetComponentLevels(map[string]slog.Level{"api": slog.LevelDebug}), "setting the component levels must not fail")
	require.NotSame(t, handler, slog.Default().Handler(), "an override enabling debug adds source information, which is a property of the handler")
}

func TestAddStacktrace(t *testing.T) {
	tests := []struct {
		name string

		attrs []any

		assertStacktrace require.ComparisonAssertionFunc
	}{
		{
			name:  "with stack trace attribute",
			attrs: []any{AddStacktrace()},

			assertStacktrace: require.Contains,
		},
		{
			name:  "without stack trace attribute",
			attrs: []any{},

			assertStacktrace: require.NotContains,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}

			require.NoError(t, InitLogger(logBuf, "", false, false, true), "logger initialization must not fail")

			// Run test
			slog.WarnContext(t.Context(), "message", tc.attrs...)

			// Assert
			tc.assertStacktrace(t, logBuf.String(), "stacktrace=", "only the stack trace attribute adds a stack trace")
			tc.assertStacktrace(t, logBuf.String(), "TestAddStacktrace", "the stack trace names the calling function")
		})
	}
}
