package logger

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/log"
)

func TestSetLevelsKeepsLevelsOnHandlerFailure(t *testing.T) {
	logBuf := &log.SyncBuffer{}

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
	logBuf := &log.SyncBuffer{}

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
			logBuf := &log.SyncBuffer{}

			require.NoError(t, InitLogger(logBuf, "", false, false, true), "logger initialization must not fail")

			// Run test
			slog.WarnContext(t.Context(), "message", tc.attrs...)

			// Assert
			tc.assertStacktrace(t, logBuf.String(), "stacktrace=", "only the stack trace attribute adds a stack trace")
			tc.assertStacktrace(t, logBuf.String(), "TestAddStacktrace", "the stack trace names the calling function")
		})
	}
}

func TestContextWithTestLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		verbose bool
		debug   bool
		run     func(ctx context.Context)

		assertLog log.MatcherFunc
	}{
		{
			name: "warning",
			run: func(ctx context.Context) {
				slog.WarnContext(ctx, "message")
			},

			assertLog: log.Match(`WRN message\n$`),
		},
		{
			name: "info is not logged by default",
			run: func(ctx context.Context) {
				slog.InfoContext(ctx, "message")
			},

			assertLog: log.Empty,
		},
		{
			name:    "info with verbose",
			verbose: true,
			run: func(ctx context.Context) {
				slog.InfoContext(ctx, "message")
			},

			assertLog: log.Match(`INF message\n$`),
		},
		{
			name:  "debug with debug",
			debug: true,
			run: func(ctx context.Context) {
				slog.DebugContext(ctx, "message")
			},

			assertLog: log.Match(`DBG .*message\n$`),
		},
		{
			name: "with attributes and group",
			run: func(ctx context.Context) {
				slog.With(slog.String("outer", "1")).WithGroup("group").With(slog.String("inner", "2")).WarnContext(ctx, "message", slog.String("attr", "3"))
			},

			assertLog: log.Match(`WRN message outer=1 group.inner=2 group.attr=3\n$`),
		},
		{
			name: "with context attribute and component",
			run: func(ctx context.Context) {
				ctx = ContextWithAttr(ctx, slog.String("request", "1"))
				ctx = ContextWithComponent(ctx, "api")

				slog.WarnContext(ctx, "message")
			},

			assertLog: log.Match(`WRN message component=api request=1\n$`),
		},
		{
			name: "detached context",
			run: func(ctx context.Context) {
				slog.WarnContext(DetachedContext(ctx), "message")
			},

			assertLog: log.Match(`WRN message\n$`),
		},
		{
			name: "with stack trace",
			run: func(ctx context.Context) {
				slog.WarnContext(ctx, "message", AddStacktrace())
			},

			assertLog: log.Contains("TestContextWithTestLogger"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			logBuf := &log.SyncBuffer{}
			ctx := ContextWithTestLogger(t, logBuf, tc.verbose, tc.debug, true)

			// Run test
			tc.run(ctx)

			// Assert
			tc.assertLog(t, logBuf)
		})
	}
}

func TestContextWithTestLoggerDoesNotWriteToDefaultLogger(t *testing.T) {
	defaultLogBuf := &log.SyncBuffer{}
	require.NoError(t, InitLogger(defaultLogBuf, "", false, false, true), "logger initialization must not fail")

	contextLogBuf := &log.SyncBuffer{}
	ctx := ContextWithTestLogger(t, contextLogBuf, false, false, true)

	slog.WarnContext(ctx, "context logger")
	slog.WarnContext(t.Context(), "default logger")

	require.Contains(t, contextLogBuf.String(), "context logger", "the context logger receives the records of its context")
	require.NotContains(t, contextLogBuf.String(), "default logger", "the context logger receives no records of other contexts")
	require.Contains(t, defaultLogBuf.String(), "default logger", "the default logger receives the records of other contexts")
	require.NotContains(t, defaultLogBuf.String(), "context logger", "the default logger receives no records of a context logger")
}
