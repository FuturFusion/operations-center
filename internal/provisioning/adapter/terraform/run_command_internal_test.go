package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_cleanEnvVars(t *testing.T) {
	tests := []struct {
		name string
		in   []string

		want []string
	}{
		{
			name: "no proxy env vars - return unmodified",
			in: []string{
				"FOO=bar",
				"BAZ=test",
			},

			want: []string{
				"FOO=bar",
				"BAZ=test",
			},
		},
		{
			name: "proxy env vars - return cleaned",
			in: []string{
				"FOO=bar",
				"HTTP_PROXY=http://127.0.0.1:8080",
				"HTTPS_PROXY=http://127.0.0.1:8080",
				"http_proxy=http://127.0.0.1:8080",
				"https_proxy=http://127.0.0.1:8080",
				"NO_PROXY=internal.example.com,internal2.example.com",
				"no_proxy=internal.example.com,internal2.example.com",
				"BAZ=test",
			},

			want: []string{
				"FOO=bar",
				"BAZ=test",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := cleanEnvVars(tc.in)

			require.ElementsMatch(t, tc.want, out)
		})
	}
}
