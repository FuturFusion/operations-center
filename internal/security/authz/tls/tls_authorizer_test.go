package authz_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	authztls "github.com/FuturFusion/operations-center/internal/security/authz/tls"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestTLS_CheckPermission(t *testing.T) {
	t.Parallel()

	const (
		fingerprint        = "aabbcc"
		metricsFingerprint = "ddeeff"
	)

	noErr := func(t *testing.T, err error) {
		t.Helper()
		require.NoError(t, err, "expect access to be granted")
	}

	forbidden := func(message string) func(t *testing.T, err error) {
		return func(t *testing.T, err error) {
			t.Helper()
			require.True(t, api.StatusErrorCheck(err, http.StatusForbidden), "expect a forbidden error, got: %v", err)
			require.ErrorContains(t, err, message, "expect the error to name the reason")
			require.False(t, authz.IsDenied(err), "expect the following authorizers to be asked")
		}
	}

	denied := func(message string) func(t *testing.T, err error) {
		return func(t *testing.T, err error) {
			t.Helper()
			require.True(t, api.StatusErrorCheck(err, http.StatusForbidden), "expect a forbidden error, got: %v", err)
			require.ErrorContains(t, err, message, "expect the error to name the reason")
			require.True(t, authz.IsDenied(err), "expect a final denial, so no following authorizer widens the access")
		}
	}

	tests := []struct {
		name                           string
		certificateFingerprints        []string
		metricsCertificateFingerprints []string
		protocol                       string
		username                       string
		entitlement                    authz.Entitlement

		assertErr func(t *testing.T, err error)
	}{
		{
			name:                    "wrong protocol",
			certificateFingerprints: []string{fingerprint},
			protocol:                api.AuthenticationMethodOIDC,
			username:                fingerprint,
			entitlement:             authz.EntitlementCanView,

			assertErr: forbidden("not compatible with authorizer"),
		},
		{
			name:                           "trusted certificate with edit entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       fingerprint,
			entitlement:                    authz.EntitlementCanEdit,

			assertErr: noErr,
		},
		{
			name:                           "trusted certificate with metrics entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       fingerprint,
			entitlement:                    authz.EntitlementCanViewMetrics,

			assertErr: noErr,
		},
		{
			name:                    "trusted certificate with colons and upper case in configuration",
			certificateFingerprints: []string{"AA:BB:CC"},
			protocol:                api.AuthenticationMethodTLS,
			username:                fingerprint,
			entitlement:             authz.EntitlementCanEdit,

			assertErr: noErr,
		},
		{
			name:                           "metrics only certificate with metrics entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       metricsFingerprint,
			entitlement:                    authz.EntitlementCanViewMetrics,

			assertErr: noErr,
		},
		{
			name:                           "metrics only certificate with colons and upper case in configuration",
			metricsCertificateFingerprints: []string{"DD:EE:FF"},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       metricsFingerprint,
			entitlement:                    authz.EntitlementCanViewMetrics,

			assertErr: noErr,
		},
		{
			name:                           "metrics only certificate with view entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       metricsFingerprint,
			entitlement:                    authz.EntitlementCanView,

			assertErr: denied("restricted to metrics access"),
		},
		{
			name:                           "metrics only certificate with edit entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       metricsFingerprint,
			entitlement:                    authz.EntitlementCanEdit,

			assertErr: denied("restricted to metrics access"),
		},
		{
			name:                           "metrics only certificate with create entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       metricsFingerprint,
			entitlement:                    authz.EntitlementCanCreate,

			assertErr: denied("restricted to metrics access"),
		},
		{
			name:                           "metrics only certificate with delete entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       metricsFingerprint,
			entitlement:                    authz.EntitlementCanDelete,

			assertErr: denied("restricted to metrics access"),
		},
		{
			name:                           "metrics only certificate with debug entitlement",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       metricsFingerprint,
			entitlement:                    authz.EntitlementCanDebug,

			assertErr: denied("restricted to metrics access"),
		},
		{
			name:                           "certificate in both lists has full access",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{fingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       fingerprint,
			entitlement:                    authz.EntitlementCanEdit,

			assertErr: noErr,
		},
		{
			name:                           "unknown certificate",
			certificateFingerprints:        []string{fingerprint},
			metricsCertificateFingerprints: []string{metricsFingerprint},
			protocol:                       api.AuthenticationMethodTLS,
			username:                       "unknown",
			entitlement:                    authz.EntitlementCanViewMetrics,

			assertErr: forbidden("Client certificate not found"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			ctx := t.Context()
			authorizer := authztls.New(ctx, tc.certificateFingerprints, authztls.WithMetricsCertificateFingerprints(tc.metricsCertificateFingerprints))
			details := &authz.RequestDetails{
				Protocol: tc.protocol,
				Username: tc.username,
			}

			// Run test
			err := authorizer.CheckPermission(ctx, details, authz.ObjectServer(), tc.entitlement)

			// Assert
			tc.assertErr(t, err)
		})
	}
}
