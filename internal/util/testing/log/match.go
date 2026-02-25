package log

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type MatcherFunc func(t *testing.T, logBuf *bytes.Buffer)

func Noop(t *testing.T, logBuf *bytes.Buffer) {
	t.Helper()
}

func Empty(t *testing.T, logBuf *bytes.Buffer) {
	t.Helper()

	require.Empty(t, logBuf.String())
}

func Contains(want string) func(t *testing.T, logBuf *bytes.Buffer) {
	return func(t *testing.T, logBuf *bytes.Buffer) {
		t.Helper()

		// Give logs a little bit of time to be processed.
		for range 5 {
			if strings.Contains(logBuf.String(), want) {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		require.Contains(t, logBuf.String(), want)
	}
}

func Match(expr string) func(t *testing.T, logBuf *bytes.Buffer) {
	return func(t *testing.T, logBuf *bytes.Buffer) {
		t.Helper()

		re, err := regexp.Compile(expr)
		require.NoError(t, err)

		// Give logs a little bit of time to be processed.
		for range 5 {
			if re.Match(logBuf.Bytes()) {
				break
			}

			time.Sleep(10 * time.Millisecond)
		}

		require.True(t, re.Match(logBuf.Bytes()), "logBuf did not match expression: %q, logBuf:\n%s", expr, logBuf.String())
	}
}
