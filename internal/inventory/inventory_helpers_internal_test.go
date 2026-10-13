package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_firstNonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		candidates []string

		want string
	}{
		{
			name: "empty",

			want: "",
		},
		{
			name:       "candidates",
			candidates: []string{"one", "two"},

			want: "one",
		},
		{
			name:       "empty candidates",
			candidates: []string{"", ""},

			want: "",
		},
		{
			name:       "empty candidates",
			candidates: []string{"", "two"},

			want: "two",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := firstNonEmpty(tc.candidates...)
			require.Equal(t, tc.want, got)
		})
	}
}
