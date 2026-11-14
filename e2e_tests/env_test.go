package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_envPositiveIntOrDefault(t *testing.T) {
	tests := []struct {
		name  string
		value string

		want int
	}{
		{
			name:  "not set",
			value: "",

			want: 4,
		},
		{
			name:  "set",
			value: "2",

			want: 2,
		},
		{
			name:  "zero",
			value: "0",

			want: 4,
		},
		{
			name:  "negative",
			value: "-1",

			want: 4,
		},
		{
			name:  "no number",
			value: "many",

			want: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			t.Setenv("OPERATIONS_CENTER_E2E_TEST_POSITIVE_INT", tc.value)

			// Run test
			got := envPositiveIntOrDefault("OPERATIONS_CENTER_E2E_TEST_POSITIVE_INT", 4)

			// Assert
			require.Equal(t, tc.want, got, "a value below 1 or no number falls back to the default")
		})
	}
}
