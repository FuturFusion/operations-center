package provisioning_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestToken_Validate(t *testing.T) {
	tests := []struct {
		name  string
		token provisioning.Token

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			token: provisioning.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr: require.NoError,
		},
		{
			name: "error - remaining uses",
			token: provisioning.Token{
				UsesRemaining: -1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - expire at",
			token: provisioning.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(-1 * time.Minute),
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
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

func TestTokenSeedConfig_Validate(t *testing.T) {
	tests := []struct {
		name            string
		tokenSeedConfig provisioning.TokenSeedConfig

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			tokenSeedConfig: provisioning.TokenSeedConfig{
				ImageType: api.ImageTypeISO,
			},

			assertErr: require.NoError,
		},
		{
			name: "valid",
			tokenSeedConfig: provisioning.TokenSeedConfig{
				ImageType: api.ImageType(`invalid`),
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tokenSeedConfig.Validate()

			tc.assertErr(t, err)
		})
	}
}
