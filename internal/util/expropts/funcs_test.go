package expropts_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/expropts"
)

func TestToFloat64(t *testing.T) {
	type specialFloat64 float64

	tests := []struct {
		name string
		in   any

		want float64
	}{
		{
			name: "float64",
			in:   float64(9.25),

			want: float64(9.25),
		},
		{
			name: "float32",
			in:   float32(9.25),

			want: float64(9.25),
		},
		{
			name: "special float 64",
			in:   specialFloat64(9.25),

			want: float64(9.25),
		},
		{
			name: "int",
			in:   int(9),

			want: float64(9),
		},
		{
			name: "uint",
			in:   uint(9),

			want: float64(9),
		},
		{
			name: "string",
			in:   "9.25",

			want: float64(0),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expropts.ToFloat64(tc.in)
			require.NoError(t, err)

			require.InDelta(t, tc.want, got, 0.01)
		})
	}
}
