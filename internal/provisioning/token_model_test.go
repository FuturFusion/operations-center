package provisioning_test

import (
	"database/sql/driver"
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
				Channel:       "stable",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - remaining uses",
			token: provisioning.Token{
				UsesRemaining: -1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Channel:       "stable",
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
				Channel:       "stable",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - channel",
			token: provisioning.Token{
				UsesRemaining: 1,
				ExpireAt:      time.Now().Add(1 * time.Minute),
				Channel:       "",
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
				Applications: api.SeedApplications{
					Version: "1",
				},
				Incus: api.SeedIncus{
					Version: "1",
				},
				Install: api.SeedInstall{
					Version: "1",
				},
				MigrationManager: api.SeedMigrationManager{
					Version: "1",
				},
				Network: api.SeedNetwork{
					Version: "1",
				},
				OperationsCenter: api.SeedOperationsCenter{
					Version: "1",
				},
				Update: api.SeedUpdate{
					Version: "1",
				},
			},

			assertErr: require.NoError,
			wantValue: []byte(`{"applications":{"version":"1","applications":null},"incus":{"version":"1","apply_defaults":false,"preseed":null},"install":{"version":"1","force_install":false,"force_reboot":false,"target":null},"migration_manager":{"version":"1","apply_defaults":false,"preseed":null},"network":{"version":"1"},"operations_center":{"version":"1","apply_defaults":false,"preseed":null},"update":{"auto_reboot":false,"channel":"","check_frequency":"","version":"1"}}`),
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

			value: []byte(`{"applications":{"version":"1"},"network":{"version":"1"},"install":{"version":"1"}}`),

			assertErr: require.NoError,
			want: provisioning.TokenImageSeedConfigs{
				Applications: api.SeedApplications{
					Version: "1",
				},
				Network: api.SeedNetwork{
					Version: "1",
				},
				Install: api.SeedInstall{
					Version: "1",
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

			value: `{"applications":{"version":"1"},"network":{"version":"1"},"install":{"version":"1"}}`,

			assertErr: require.NoError,
			want: provisioning.TokenImageSeedConfigs{
				Applications: api.SeedApplications{
					Version: "1",
				},
				Network: api.SeedNetwork{
					Version: "1",
				},
				Install: api.SeedInstall{
					Version: "1",
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

func TestTokenSeed_Validate(t *testing.T) {
	tests := []struct {
		name      string
		tokenSeed provisioning.TokenSeed

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			tokenSeed: provisioning.TokenSeed{
				Name: "name",
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			tokenSeed: provisioning.TokenSeed{
				Name: "", // invalid, name empty
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tokenSeed.Validate()

			tc.assertErr(t, err)
		})
	}
}
