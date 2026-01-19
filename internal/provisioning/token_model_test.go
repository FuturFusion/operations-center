package provisioning_test

import (
	"database/sql/driver"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
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

func TestTokenImageSeedConfigs_Value(t *testing.T) {
	tests := []struct {
		name string

		tokenImageSeedConfigs provisioning.TokenImageSeedConfigs

		assertErr require.ErrorAssertionFunc
		wantValue driver.Value
	}{
		{
			name: "success",

			tokenImageSeedConfigs: provisioning.TokenImageSeedConfigs{
				Applications: map[string]any{
					"applications": true,
				},
				Incus: map[string]any{
					"incus": true,
				},
				Install: map[string]any{
					"install": true,
				},
				MigrationManager: map[string]any{
					"migration_manager": true,
				},
				Network: map[string]any{
					"network": true,
				},
				OperationsCenter: map[string]any{
					"operations_center": true,
				},
				Update: map[string]any{
					"update": true,
				},
			},

			assertErr: require.NoError,
			wantValue: []byte(`{"applications":{"applications":true},"incus":{"incus":true},"install":{"install":true},"migration_manager":{"migration_manager":true},"network":{"network":true},"operations_center":{"operations_center":true},"update":{"update":true}}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.tokenImageSeedConfigs.Value()

			tc.assertErr(t, err)
			require.Equal(t, tc.wantValue, got)
		})
	}
}

func TestTokenImageSeeds_Scan(t *testing.T) {
	tests := []struct {
		name string

		value any

		assertErr require.ErrorAssertionFunc
		want      provisioning.TokenImageSeedConfigs
	}{
		{
			name: "success - []byte",

			value: []byte(`{"applications":{"applications":true},"network":{"network":true},"install":{"install":true}}`),

			assertErr: require.NoError,
			want: provisioning.TokenImageSeedConfigs{
				Applications: map[string]any{
					"applications": true,
				},
				Network: map[string]any{
					"network": true,
				},
				Install: map[string]any{
					"install": true,
				},
			},
		},
		{
			name: "success - []byte zero length",

			value: []byte(``),

			assertErr: require.NoError,
			want:      provisioning.TokenImageSeedConfigs{},
		},
		{
			name: "success - string",

			value: `{"applications":{"applications":true},"network":{"network":true},"install":{"install":true}}`,

			assertErr: require.NoError,
			want: provisioning.TokenImageSeedConfigs{
				Applications: map[string]any{
					"applications": true,
				},
				Network: map[string]any{
					"network": true,
				},
				Install: map[string]any{
					"install": true,
				},
			},
		},
		{
			name: "success - string zero length",

			value: ``,

			assertErr: require.NoError,
			want:      provisioning.TokenImageSeedConfigs{},
		},
		{
			name: "error - nil",

			assertErr: require.Error,
			want:      provisioning.TokenImageSeedConfigs{},
		},
		{
			name: "error - unsupported type",

			value: 1, // not supported for TokenImageSeeds

			assertErr: require.Error,
			want:      provisioning.TokenImageSeedConfigs{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenImageSeeds := provisioning.TokenImageSeedConfigs{}

			err := tokenImageSeeds.Scan(tc.value)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, tokenImageSeeds)
		})
	}
}
