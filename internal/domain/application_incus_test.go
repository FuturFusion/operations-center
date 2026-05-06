package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func TestIsApplicationNameIncusKind(t *testing.T) {
	tests := []struct {
		name string

		want bool
	}{
		{
			name: "incus",

			want: true,
		},
		{
			name: "incus-lts-7.0",

			want: true,
		},
		{
			name: "operations-center",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.IsApplicationNameIncusKind(tc.name)
			require.Equal(t, tc.want, got)
		})
	}
}
