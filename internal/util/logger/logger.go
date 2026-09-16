// Package logger provides a preinitialized slog logger ready for use.
//
// Log levels are configurable per component, see Component. The level filtering
// is performed by contextHandler, which has access to the context and therefore
// to the component a record belongs to. The sink handler accepts every level.
//
// Additionally it provides middlewares for use with http handlers to record
// access logs.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fabien-marty/slog-helpers/pkg/stacktrace"
	"github.com/lmittmann/tint"
)

const (
	LevelTrace            slog.Level = -8
	LevelTraceString                 = "TRACE"
	levelTraceStringShort            = "TRC"
)

const MaximumValueLength = 2000

type loggerContainer struct {
	writer   io.Writer
	filepath string
	file     *os.File
	noColor  bool

	// initialized reports whether a handler has been installed already.
	initialized bool

	// debug reports whether the installed handler was built with source
	// information and value truncation enabled.
	debug bool
}

var (
	logger   loggerContainer
	loggerMu sync.Mutex
)

func InitLogger(writer io.Writer, filepath string, verbose bool, debug bool, noColor bool) error {
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
	defer loggerMu.Unlock()

	logger = loggerContainer{
		writer:   writer,
		filepath: filepath,
		noColor:  noColor,
	}

	return setLevels(level, nil)
}

// SetLogLevel sets the default log level, which applies to all components
// without a more specific log level configured.
func SetLogLevel(level slog.Level) error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	return setLevels(level, levels.Load().overrides)
}

// SetComponentLevels replaces the log levels configured for individual
// components. A level configured for a component applies to all of its children
// as well, the most specific configuration wins.
func SetComponentLevels(componentLevels map[string]slog.Level) error {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	err := setLevels(levels.Load().defaultLevel, componentLevels)
	if err != nil {
		return err
	}

	// Logged at error level, so the default log level carried by the very same
	// configuration can not silence it.
	unknown := unknownComponents(slices.Sorted(maps.Keys(componentLevels)))
	if len(unknown) > 0 {
		slog.ErrorContext(context.Background(), "Log levels configured for unknown components", slog.Any("components", unknown))
	}

	return nil
}

// setLevels puts the given log levels into effect. The handler is only rebuilt,
// if the change affects how records are rendered.
//
// It must be called with loggerMu held.
func setLevels(defaultLevel slog.Level, overrides map[string]slog.Level) error {
	ls := newLevelSet(defaultLevel, overrides)

	// Source information and the truncation of values are properties of the
	// handler, which can not be decided per record, so they are derived from
	// the most verbose level in effect anywhere.
	debug := ls.min <= slog.LevelDebug
	if !logger.initialized || logger.debug != debug {
		err := installHandler(debug)
		if err != nil {
			return err
		}
	}

	levels.Store(ls)

	return nil
}

// installHandler builds the handler chain and installs it as default logger.
//
// The sink handler is configured to accept every level, the filtering by level
// is performed by contextHandler.
//
// It must be called with loggerMu held.
func installHandler(debug bool) error {
	if logger.writer == nil {
		logger.writer = os.Stderr
	}

	var slogHandler slog.Handler

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
				Level: LevelTrace,
				// Add source information, if debug level is enabled.
				AddSource:   debug,
				ReplaceAttr: replaceAttr(LevelTraceString, debug),
			},
		)
	} else {
		slogHandler = tint.NewTextHandler(logger.writer, &tint.Options{
			Level:       LevelTrace,
			TimeFormat:  time.RFC3339,
			AddSource:   debug,
			ReplaceAttr: replaceAttr(levelTraceStringShort, debug),
			NoColor:     logger.noColor,
		})
	}

	slog.SetDefault(slog.New(
		newContextHandler(stacktrace.New(slogHandler, &stacktrace.Options{
			Mode:                                    stacktrace.ModeAddAttr,
			MinimalLevelForStackTraceEnabledEnabled: new(slog.Level(100)), // Disable automatic addition of stack traces
		})),
	))

	logger.initialized = true
	logger.debug = debug

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

// replaceAttr is a slog.ReplaceAttr function, which renders LevelTrace using
// the given name and, if debug is enabled, limits the size of logged values.
func replaceAttr(traceName string, debug bool) func(groups []string, attr slog.Attr) slog.Attr {
	truncate := logValueMaxSize(MaximumValueLength)

	return func(groups []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.LevelKey && len(groups) == 0 {
			level, ok := attr.Value.Any().(slog.Level)
			if ok && level == LevelTrace {
				attr.Value = slog.StringValue(traceName)
			}

			return attr
		}

		if !debug {
			return attr
		}

		return truncate(groups, attr)
	}
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
//
// It is also the handler performing the filtering by log level.
type contextHandler struct {
	slog.Handler
}

// newContextHandler creates a new slog context handler.
func newContextHandler(handler slog.Handler) *contextHandler {
	return &contextHandler{
		Handler: handler,
	}
}

// Enabled overwrites the Enabled method from the embedded slog.Handler. The
// level to check against is resolved from the component the context is scoped
// to, which allows different parts of the system to be logged at different
// levels.
func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return levels.Load().enabled(ctx, level)
}

// Handle overwrites the Handle method from the embedded slog.Handler.
func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	component, ok := ComponentFromContext(ctx)
	if ok {
		r.Add(slog.String(componentContextKey, string(component)))
	}

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

// WithGroup overwrites the WithGroup method from the embedded slog.Handler, so
// the derived handler keeps filtering by component.
func (h contextHandler) WithGroup(name string) slog.Handler {
	h.Handler = h.Handler.WithGroup(name)
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

// DetachedContext returns a detached context where logging related context
// values are taken over.
func DetachedContext(ctx context.Context) context.Context {
	detached := context.WithValue(context.Background(), contextHandlerKey{}, ctx.Value(contextHandlerKey{}))

	component, ok := ComponentFromContext(ctx)
	if ok {
		detached = ContextWithComponent(detached, component)
	}

	return detached
}

// Err is a helper function to ensure errors are always logged with the key
// "err". Additionally this becomes the single point in code, where we could
// tweak how errors are logged, e.g. to handle application specific error types
// or to add stack trace information in debug mode.
func Err(err error) slog.Attr {
	return slog.Any("err", err)
}

func AddStacktrace() slog.Attr {
	return slog.Bool("add_stacktrace", true)
}
