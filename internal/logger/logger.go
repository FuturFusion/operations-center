package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type Logger interface {
	Debug(msg string, args ...any)
	DebugContext(ctx context.Context, msg string, args ...any)
	Error(msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
	Info(msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	Warn(msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	With(args ...any) *slog.Logger
}

func InitLogger(writer io.Writer, filepath string, verbose bool, debug bool) error {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelInfo
	}

	if debug {
		level = slog.LevelDebug
	}

	if filepath != "" {
		f, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}

		writer = io.MultiWriter(writer, f)
	}

	logger := slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: level,
		// Add source information, if debug level is enabled.
		AddSource: debug,
	}))

	slog.SetDefault(logger)

	return nil
}

// Err is a helper function to ensure errors are always logged with the key
// "err". Additionally this becomes the single point in code, where we could
// tweak how errors are logged, e.g. to handle application specific error types
// or to add stack trace information in debug mode.
func Err(err error) slog.Attr {
	return slog.Any("err", err)
}
