package acme_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/acme"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func TestIsEnabled(t *testing.T) {
	t.Parallel()

	complete := system.SecurityACME{
		AgreeTOS: true,
		CAURL:    "https://acme-v02.api.letsencrypt.org/directory",
		Domain:   "operations-center.example.org",
		Email:    "admin@example.org",
	}

	tests := []struct {
		name   string
		modify func(cfg system.SecurityACME) system.SecurityACME

		want bool
	}{
		{
			name:   "complete",
			modify: func(cfg system.SecurityACME) system.SecurityACME { return cfg },

			want: true,
		},
		{
			name: "terms of service not agreed",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.AgreeTOS = false
				return cfg
			},

			want: false,
		},
		{
			name: "domain missing",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.Domain = ""
				return cfg
			},

			want: false,
		},
		{
			name: "email missing",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.Email = ""
				return cfg
			},

			want: false,
		},
		{
			name: "CA URL missing",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.CAURL = ""
				return cfg
			},

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := acme.IsEnabled(tc.modify(complete))

			require.Equal(t, tc.want, got, "ACME enabled state")
		})
	}
}

func TestValidateACMEConfig(t *testing.T) {
	t.Parallel()

	valid := system.SecurityACME{
		AgreeTOS:  true,
		CAURL:     "https://acme-v02.api.letsencrypt.org/directory",
		Domain:    "operations-center.example.org",
		Email:     "admin@example.org",
		Address:   ":80",
		Challenge: "HTTP-01",
	}

	tests := []struct {
		name   string
		modify func(cfg system.SecurityACME) system.SecurityACME

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:   "valid",
			modify: func(cfg system.SecurityACME) system.SecurityACME { return cfg },

			assertErr: require.NoError,
		},
		{
			name:   "valid - not configured",
			modify: func(cfg system.SecurityACME) system.SecurityACME { return system.SecurityACME{} },

			assertErr: require.NoError,
		},
		{
			name: "error - CA URL host is parent directory",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.CAURL = "https://../directory"
				return cfg
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `ACME CA URL host ".." is invalid`, a...)
			},
		},
		{
			name: "error - email with path separator",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.Email = "../../escape@example.org"
				return cfg
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `ACME email "../../escape@example.org" is not a valid email address`, a...)
			},
		},
		{
			name: "error - email with path separator in local part",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.Email = "a/b@example.org"
				return cfg
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `ACME email "a/b@example.org" is not a valid email address`, a...)
			},
		},
		{
			name: "error - email with display name",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.Email = "Admin <admin@example.org>"
				return cfg
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "is not a valid email address", a...)
			},
		},
		{
			name: "error - email invalid",
			modify: func(cfg system.SecurityACME) system.SecurityACME {
				cfg.Email = "invalid"
				return cfg
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "is not a valid email address", a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			err := acme.ValidateACMEConfig(tc.modify(valid))

			// Assert
			tc.assertErr(t, err)
		})
	}
}
