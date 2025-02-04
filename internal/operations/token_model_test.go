package operations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/operations"
)

func TestToken_Validate(t *testing.T) {
	tests := []struct {
		name  string
		token operations.Token

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			token: operations.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr: require.NoError,
		},
		{
			name: "error - remaining uses",
			token: operations.Token{
				UsesRemaining: -1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				var verr operations.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
		{
			name: "error - expire at",
			token: operations.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(-1 * time.Minute),
			},

			assertErr: func(tt require.TestingT, err error, i ...any) {
				var verr operations.ErrValidation
				require.ErrorAs(tt, err, &verr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.token.Validate()

			tc.assertErr(t, err)
		})
	}
}
