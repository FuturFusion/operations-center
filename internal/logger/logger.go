// Package logger provides a preinitialized slog logger ready for use.
//
// Additionally it provides middlewares for use with http handlers to record
// access logs.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lmittmann/tint"
)

const (
	LevelTrace       slog.Level = -8
	LevelTraceString            = "TRACE"
)

const MaximumValueLength = 2000

type loggerContainer struct {
	writer   io.Writer
	filepath string
	file     *os.File
}

var (
	logger   loggerContainer
	loggerMu sync.Mutex
)

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

	loggerMu.Lock()
	logger = loggerContainer{
		writer:   writer,
		filepath: filepath,
	}

	loggerMu.Unlock()

	return SetLogLevel(level)
}

// SetLogLevel replaces the default logger with a logger of the given log level.
func SetLogLevel(level slog.Level) error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	var replaceAttrFunc func(groups []string, attr slog.Attr) slog.Attr

	var debug bool
	if level <= slog.LevelDebug {
		debug = true
		replaceAttrFunc = logValueMaxSize(MaximumValueLength)
	}

	slogHandler := tint.NewHandler(
		logger.writer,
		&tint.Options{
			Level:      level,
			TimeFormat: time.RFC3339,
			// Add source information, if debug level is enabled.
			AddSource:   debug,
			ReplaceAttr: replaceAttrFunc,
		},
	)

	if logger.filepath != "" {
		if logger.file == nil {
			var err error
			logger.file, err = os.OpenFile(logger.filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
			if err != nil {
				return err
			}

			logger.writer = io.MultiWriter(logger.writer, logger.file)
		}

		slogHandler = slog.NewTextHandler(
			logger.writer,
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

// ValidateLevel checks a given string representation for a log level against
// the supported log levels.
func ValidateLevel(levelStr string) error {
	// Empty string is ok, we just use the default value in this cases.
	if levelStr == "" {
		return nil
	}

	validLogLevels := []string{LevelTraceString, slog.LevelDebug.String(), slog.LevelInfo.String(), slog.LevelWarn.String(), slog.LevelError.String()}
	if !slices.Contains(validLogLevels, levelStr) {
		return fmt.Errorf("Log level %q is invalid, must be one of %q", levelStr, strings.Join(validLogLevels, ","))
	}

	return nil
}

// ParseLevel converts a given string representation for a log level into
// an actual slog.Level.
// Defaults to log level warn.
func ParseLevel(levelStr string) slog.Level {
	level := slog.LevelWarn

	switch levelStr {
	case LevelTraceString:
		level = LevelTrace
	case slog.LevelDebug.String():
		level = slog.LevelDebug
	case slog.LevelInfo.String():
		level = slog.LevelInfo
	case slog.LevelWarn.String():
		level = slog.LevelWarn
	case slog.LevelError.String():
		level = slog.LevelError
	}

	return level
}

// logValueMaxSize is a slog.ReplaceAttr function, which limits the size
// of log values to the given limit.
func logValueMaxSize(limit int) func(groups []string, attr slog.Attr) slog.Attr {
	return func(groups []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.LevelKey || attr.Key == slog.SourceKey {
			return attr
		}

		switch attr.Value.Kind() {
		case slog.KindAny, slog.KindString:
			if attr.Key == slog.MessageKey {
				break
			}

			val := attr.Value.String()
			if len(val) > limit {
				val = val[:limit] + "... (truncated)"
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

// WithAttrs overwirtes the WithAttrs method from the embedded slog.Handler.
func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.Handler = h.Handler.WithAttrs(attrs)
	return h
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
