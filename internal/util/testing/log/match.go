package log

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/stretchr/testify/require"
)

type TestifyT interface {
	require.TestingT
	Helper()
}

type MatcherFunc func(t TestifyT, logBuf fmt.Stringer)

func Noop(t TestifyT, logBuf fmt.Stringer) {
	t.Helper()
}

func Empty(t TestifyT, logBuf fmt.Stringer) {
	t.Helper()

	require.Empty(t, logBuf.String())
}

var IgnorePatternDebugLines = "[0-9TZ:-]+ DBG.*"

func EmptyWithIgnorePattern(pattern string) func(t TestifyT, logBuf fmt.Stringer) {
	return func(t TestifyT, logBuf fmt.Stringer) {
		t.Helper()

		re, err := regexp.Compile(pattern)
		require.NoError(t, err)

		lines := strings.Split(logBuf.String(), "\n")

		out := lines[:0]

		for _, line := range lines {
			if re.MatchString(line) {
				continue
			}

			out = append(out, line)
		}

		filteredLog := strings.Join(out, "\n")

		require.Empty(t, filteredLog)
	}
}

func Contains(want string) func(t TestifyT, logBuf fmt.Stringer) {
	return func(t TestifyT, logBuf fmt.Stringer) {
		t.Helper()

		// Give logs a little bit of time to be processed.
		for range 20 {
			if strings.Contains(logBuf.String(), want) {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		require.Contains(t, logBuf.String(), want)
	}
}

func NotContains(unwanted string) func(t TestifyT, logBuf fmt.Stringer) {
	return func(t TestifyT, logBuf fmt.Stringer) {
		t.Helper()

		require.NotContains(t, logBuf.String(), unwanted)
	}
}

func Match(expr string) func(t TestifyT, logBuf fmt.Stringer) {
	return func(t TestifyT, logBuf fmt.Stringer) {
		t.Helper()

		re, err := regexp.Compile(expr)
		require.NoError(t, err)

		// Give logs a little bit of time to be processed.
		for range 20 {
			if re.MatchString(logBuf.String()) {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		require.True(t, re.MatchString(logBuf.String()), "logBuf did not match expression: %q, logBuf:\n%s", expr, logBuf.String())
	}
}
