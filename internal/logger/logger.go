package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

const LevelTrace slog.Level = -8

func InitLogger(writer io.Writer, filepath string, verbose bool, debug bool) error {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelInfo
	}

	if debug {
		level = slog.LevelDebug
	}

	if verbose && debug {
		level = LevelTrace
	}

	slogHandler := tint.NewHandler(
		writer,
		&tint.Options{
			Level:      level,
			TimeFormat: time.RFC3339,
			// Add source information, if debug level is enabled.
			AddSource: debug,
		},
	)

	if filepath != "" {
		f, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}

		writer = io.MultiWriter(writer, f)

		slogHandler = slog.NewTextHandler(
			writer,
			&slog.HandlerOptions{
				Level: level,
				// Add source information, if debug level is enabled.
				AddSource: debug,
			},
		)
	}

	logger := slog.New(
		NewContextHandler(slogHandler),
	)

	slog.SetDefault(logger)

	return nil
}

type contextHandler struct {
	slog.Handler
}

func NewContextHandler(handler slog.Handler) *contextHandler {
	return &contextHandler{
		Handler: handler,
	}
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs, ok := ctx.Value(contextHandlerKey{}).(*[]slog.Attr)
	if ok {
		for _, a := range *attrs {
			a.Value = a.Value.Resolve()
			r.Add(a)
		}
	}

	return h.Handler.Handle(ctx, r)
}

type contextHandlerKey struct{}

func ContextWithAttr(ctx context.Context, attr slog.Attr) context.Context {
	attrs, ok := ctx.Value(contextHandlerKey{}).(*[]slog.Attr)
	if !ok {
		attrs = new([]slog.Attr)
	}

	*attrs = append(*attrs, attr)

	return context.WithValue(ctx, contextHandlerKey{}, attrs)
}

// Err is a helper function to ensure errors are always logged with the key
// "err". Additionally this becomes the single point in code, where we could
// tweak how errors are logged, e.g. to handle application specific error types
// or to add stack trace information in debug mode.
func Err(err error) slog.Attr {
	return slog.Any("err", err)
}
