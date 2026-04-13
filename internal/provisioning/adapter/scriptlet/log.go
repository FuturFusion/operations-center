package scriptlet

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
)

// CreateLogger creates a logger for scriptlets.
func CreateLogger(l *slog.Logger, name string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var sb strings.Builder
		for _, arg := range args {
			s, err := strconv.Unquote(arg.String())
			if err != nil {
				s = arg.String()
			}

			sb.WriteString(s)
		}

		switch b.Name() {
		case "info":
			l.Info(fmt.Sprintf("%s: %s", name, sb.String())) //nolint:sloglint

		case "warn":
			l.Warn(fmt.Sprintf("%s: %s", name, sb.String())) //nolint:sloglint

		default:
			l.Error(fmt.Sprintf("%s: %s", name, sb.String())) //nolint:sloglint
		}

		return starlark.None, nil
	}
}
