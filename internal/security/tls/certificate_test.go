package tls_test

import (
	"testing"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
)

func TestCertificateFingerprints(t *testing.T) {
	tests := []struct {
		name            string
		certificatesPEM []string

		assertErr require.ErrorAssertionFunc
		want      []string
	}{
		{
			name:            "success - nil",
			certificatesPEM: nil,

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:            "success - single certificate",
			certificatesPEM: []string{testcert.ClientCertificate},

			assertErr: require.NoError,
			want:      []string{testcert.ClientCertificateFingerprint},
		},
		{
			name:            "success - same certificate provided twice",
			certificatesPEM: []string{testcert.ClientCertificate, testcert.ClientCertificate},

			assertErr: require.NoError,
			want:      []string{testcert.ClientCertificateFingerprint},
		},
		{
			name:            "error - not a PEM block",
			certificatesPEM: []string{"not a certificate"},

			assertErr: require.Error,
			want:      []string{},
		},
		{
			name: "error - not an X509 certificate",
			certificatesPEM: []string{`-----BEGIN CERTIFICATE-----
Zm9vYmFy
-----END CERTIFICATE-----`},

			assertErr: require.Error,
			want:      []string{},
		},
		{
			name:            "error - invalid certificate does not drop the valid ones",
			certificatesPEM: []string{"not a certificate", testcert.ClientCertificate},

			assertErr: require.Error,
			want:      []string{testcert.ClientCertificateFingerprint},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := securitytls.CertificateFingerprints(tc.certificatesPEM)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTrustedClientCertificates(t *testing.T) {
	wantCertificate := incusapi.CertificatesPost{
		CertificatePut: incusapi.CertificatePut{
			Name:        "oc-trusted-" + testcert.ClientCertificateFingerprint[:12],
			Description: "Client trusted by Operations Center",
			Type:        "client",
			Restricted:  false,
			Projects:    []string{},
			Certificate: testcert.ClientCertificate,
		},
	}

	tests := []struct {
		name            string
		certificatesPEM []string

		assertErr require.ErrorAssertionFunc
		want      []incusapi.CertificatesPost
	}{
		{
			name:            "success - nil",
			certificatesPEM: nil,

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:            "success - single certificate",
			certificatesPEM: []string{testcert.ClientCertificate},

			assertErr: require.NoError,
			want:      []incusapi.CertificatesPost{wantCertificate},
		},
		{
			name:            "success - same certificate provided twice",
			certificatesPEM: []string{testcert.ClientCertificate, testcert.ClientCertificate},

			assertErr: require.NoError,
			want:      []incusapi.CertificatesPost{wantCertificate},
		},
		{
			name:            "error - not a PEM block",
			certificatesPEM: []string{"not a certificate"},

			assertErr: require.Error,
			want:      nil,
		},
		{
			name: "error - not an X509 certificate",
			certificatesPEM: []string{`-----BEGIN CERTIFICATE-----
Zm9vYmFy
-----END CERTIFICATE-----`},

			assertErr: require.Error,
			want:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := securitytls.TrustedClientCertificates(tc.certificatesPEM)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTrustedClientCertificatesWithFingerprint(t *testing.T) {
	wantCertificate := securitytls.TrustedClientCertificate{
		CertificatesPost: incusapi.CertificatesPost{
			CertificatePut: incusapi.CertificatePut{
				Name:        "oc-trusted-" + testcert.ClientCertificateFingerprint[:12],
				Description: "Client trusted by Operations Center",
				Type:        "client",
				Restricted:  false,
				Projects:    []string{},
				Certificate: testcert.ClientCertificate,
			},
		},
		Fingerprint: testcert.ClientCertificateFingerprint,
	}

	tests := []struct {
		name            string
		certificatesPEM []string

		assertErr require.ErrorAssertionFunc
		want      []securitytls.TrustedClientCertificate
	}{
		{
			name:            "success - nil",
			certificatesPEM: nil,

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:            "success - single certificate",
			certificatesPEM: []string{testcert.ClientCertificate},

			assertErr: require.NoError,
			want:      []securitytls.TrustedClientCertificate{wantCertificate},
		},
		{
			name:            "success - same certificate provided twice",
			certificatesPEM: []string{testcert.ClientCertificate, testcert.ClientCertificate},

			assertErr: require.NoError,
			want:      []securitytls.TrustedClientCertificate{wantCertificate},
		},
		{
			name:            "error - not a PEM block",
			certificatesPEM: []string{"not a certificate"},

			assertErr: require.Error,
			want:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := securitytls.TrustedClientCertificatesWithFingerprint(tc.certificatesPEM)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestFilterCertificatesByFingerprints(t *testing.T) {
	tests := []struct {
		name            string
		certificatesPEM []string
		fingerprints    []string
		matching        bool

		assertErr require.ErrorAssertionFunc
		want      []string
	}{
		{
			name:            "success - nil",
			certificatesPEM: nil,

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:            "success - not matching fingerprint is kept",
			certificatesPEM: []string{testcert.ClientCertificate},
			fingerprints:    []string{"1234567890"},
			matching:        false,

			assertErr: require.NoError,
			want:      []string{testcert.ClientCertificate},
		},
		{
			name:            "success - matching fingerprint is dropped",
			certificatesPEM: []string{testcert.ClientCertificate},
			fingerprints:    []string{"1234567890", testcert.ClientCertificateFingerprint},
			matching:        false,

			assertErr: require.NoError,
			want:      []string{},
		},
		{
			name:            "success - matching fingerprint is kept",
			certificatesPEM: []string{testcert.ClientCertificate},
			fingerprints:    []string{"1234567890", testcert.ClientCertificateFingerprint},
			matching:        true,

			assertErr: require.NoError,
			want:      []string{testcert.ClientCertificate},
		},
		{
			name:            "success - not matching fingerprint is dropped",
			certificatesPEM: []string{testcert.ClientCertificate},
			fingerprints:    []string{"1234567890"},
			matching:        true,

			assertErr: require.NoError,
			want:      []string{},
		},
		{
			name:            "error - not a PEM block",
			certificatesPEM: []string{"not a certificate"},

			assertErr: require.Error,
			want:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := securitytls.FilterCertificatesByFingerprints(tc.certificatesPEM, tc.fingerprints, tc.matching)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
