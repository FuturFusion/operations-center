package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsRecursionRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string

		want bool
	}{
		{name: "missing", query: "", want: false},
		{name: "zero", query: "?recursion=0", want: false},
		{name: "one", query: "?recursion=1", want: true},
		{name: "two", query: "?recursion=2", want: true},
		{name: "negative", query: "?recursion=-1", want: false},
		{name: "invalid", query: "?recursion=abc", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/1.0/test"+tc.query, http.NoBody)

			got := isRecursionRequest(r)

			require.Equal(t, tc.want, got, "query %q", tc.query)
		})
	}
}

func Test_recursionLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string

		want int
	}{
		{name: "missing", query: "", want: recursionNone},
		{name: "invalid", query: "?recursion=abc", want: recursionNone},
		{name: "negative", query: "?recursion=-1", want: recursionNone},
		{name: "zero", query: "?recursion=0", want: recursionNone},
		{name: "one", query: "?recursion=1", want: recursionObjects},
		{name: "two", query: "?recursion=2", want: recursionFull},
		{name: "three", query: "?recursion=3", want: recursionFull},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/1.0/test"+tc.query, http.NoBody)

			got := recursionLevel(r)

			require.Equal(t, tc.want, got, "query %q", tc.query)
		})
	}
}
