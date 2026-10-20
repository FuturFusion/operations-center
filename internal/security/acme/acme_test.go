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
