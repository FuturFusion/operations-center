// Package logger provides a preinitialized slog logger ready for use.
//
// Additionally it provides middlewares for use with http handlers to record
// access logs.
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

const MaximumValueLength = 100

func InitLogger(writer io.Writer, filepath string, verbose bool, debug bool) error {
	level := slog.LevelWarn
	var replaceAttrFunc func(groups []string, attr slog.Attr) slog.Attr

	if verbose {
		level = slog.LevelInfo
	}

	if debug {
		level = slog.LevelDebug
		replaceAttrFunc = logValueMaxSize(MaximumValueLength)
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
			AddSource:   debug,
			ReplaceAttr: replaceAttrFunc,
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
				AddSource:   debug,
				ReplaceAttr: replaceAttrFunc,
			},
		)
	}

	logger := slog.New(
		newContextHandler(slogHandler),
	)

	slog.SetDefault(logger)

	return nil
}

// logValueMaxSize is a slog.ReplaceAttr function, which limits the size
// of log values to the given limit.
func logValueMaxSize(limit int) func(groups []string, attr slog.Attr) slog.Attr {
	return func(groups []string, attr slog.Attr) slog.Attr {
		switch attr.Value.Kind() {
		case slog.KindAny, slog.KindString:
			val := attr.Value.String()
			if len(val) > limit {
				val = val[:limit]
			}

			attr.Value = slog.StringValue(val)
		}

		return attr
	}
}

// contextHandler is a slog.Handler, which extracts slog attributes from
// the provided context, which have been added to the context before using
// ContextWithAttr.
type contextHandler struct {
	slog.Handler
}

// newContextHandler creates a new slog context handler.
func newContextHandler(handler slog.Handler) *contextHandler {
	return &contextHandler{
		Handler: handler,
	}
}

// Handle overwrites the Handle method from the embedded slog.Handler.
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

// ContextWithAttr returns a copy of parent in which the attr is added to the list
// of slog attributes attached to the context.
//
// Use context slog attributes only for request-scoped log attributes.
func ContextWithAttr(parent context.Context, attr slog.Attr) context.Context {
	attrs, ok := parent.Value(contextHandlerKey{}).(*[]slog.Attr)
	if !ok {
		attrs = new([]slog.Attr)
	}

	*attrs = append(*attrs, attr)

	return context.WithValue(parent, contextHandlerKey{}, attrs)
}

// Err is a helper function to ensure errors are always logged with the key
// "err". Additionally this becomes the single point in code, where we could
// tweak how errors are logged, e.g. to handle application specific error types
// or to add stack trace information in debug mode.
func Err(err error) slog.Attr {
	return slog.Any("err", err)
}
