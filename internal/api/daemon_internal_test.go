package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/certs"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_validateServerCertificate(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// 61 of 90 days passed. Within 30 days of expiration, but before the ACME renewal at 80%.
	beforeACMERenewal := certs.GenerateWithValidity(t, now.Add(-61*24*time.Hour), now.Add(29*24*time.Hour))
	// 82 of 90 days passed. The ACME renewal is overdue.
	acmeRenewalOverdue := certs.GenerateWithValidity(t, now.Add(-82*24*time.Hour), now.Add(8*24*time.Hour))

	acmeEnabled := apisystem.SecurityACME{
		AgreeTOS: true,
		CAURL:    "https://acme-v02.api.letsencrypt.org/directory",
		Domain:   "operations-center.example.org",
		Email:    "admin@example.org",
	}

	tests := []struct {
		name    string
		server  provisioning.Server
		acmeCfg apisystem.SecurityACME

		wantValid bool
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "operations center with ACME - before renewal",
			server: provisioning.Server{
				Type:        api.ServerTypeOperationsCenter,
				Certificate: &beforeACMERenewal,
			},
			acmeCfg: acmeEnabled,

			wantValid: true,
			assertErr: require.NoError,
		},
		{
			name: "operations center with ACME - renewal overdue",
			server: provisioning.Server{
				Type:        api.ServerTypeOperationsCenter,
				Certificate: &acmeRenewalOverdue,
			},
			acmeCfg: acmeEnabled,

			wantValid: true,
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "expires within", a...)
			},
		},
		{
			name: "operations center without ACME - fixed threshold",
			server: provisioning.Server{
				Type:        api.ServerTypeOperationsCenter,
				Certificate: &beforeACMERenewal,
			},
			acmeCfg: apisystem.SecurityACME{},

			wantValid: true,
			assertErr: require.Error,
		},
		{
			name: "incus server with ACME - fixed threshold",
			server: provisioning.Server{
				Type:        api.ServerTypeIncus,
				Certificate: &beforeACMERenewal,
			},
			acmeCfg: acmeEnabled,

			wantValid: true,
			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			valid, err := validateServerCertificate(tc.server, tc.acmeCfg)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantValid, valid, "Validity of the certificate")
		})
	}
}
