package api

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsRecursionRequest(t *testing.T) {
	tests := []struct {
		name  string
		query string

		want bool
	}{
		{name: "missing", query: "", want: false},
		{name: "zero", query: "?recursion=0", want: false},
		{name: "one", query: "?recursion=1", want: true},
		{name: "two", query: "?recursion=2", want: true},
		{name: "negative", query: "?recursion=-1", want: true},
		{name: "invalid", query: "?recursion=abc", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest("GET", "/1.0/test"+tc.query, nil)

			got := isRecursionRequest(r)

			require.Equal(t, tc.want, got, "query %q", tc.query)
		})
	}
}
