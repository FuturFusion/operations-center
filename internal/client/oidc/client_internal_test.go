package oidc

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestPinTrustTuple(t *testing.T) {
	t.Parallel()

	full := OIDCTrustTuple{
		Issuer:   "https://idp.example",
		ClientID: "device",
		Audience: "audience",
	}

	tests := []struct {
		name   string
		stored OIDCTrustTuple
		header http.Header

		assertErr      require.ErrorAssertionFunc
		wantTrustTuple OIDCTrustTuple
		wantChanged    bool
	}{
		{
			name:   "success - nothing stored, full tuple received",
			stored: OIDCTrustTuple{},
			header: trustTupleHeader("https://idp.example", "device", "audience"),

			assertErr:      require.NoError,
			wantTrustTuple: full,
			wantChanged:    true,
		},
		{
			name:   "success - nothing stored, nothing received",
			stored: OIDCTrustTuple{},
			header: http.Header{},

			assertErr:      require.NoError,
			wantTrustTuple: OIDCTrustTuple{},
			wantChanged:    false,
		},
		{
			name:   "success - nothing stored, issuer received",
			stored: OIDCTrustTuple{},
			header: trustTupleHeader("https://idp.example", "", ""),

			assertErr:      require.NoError,
			wantTrustTuple: OIDCTrustTuple{Issuer: "https://idp.example"},
			wantChanged:    true,
		},
		{
			name:   "success - stored, same tuple received",
			stored: full,
			header: trustTupleHeader("https://idp.example", "device", "audience"),

			assertErr:      require.NoError,
			wantTrustTuple: full,
			wantChanged:    false,
		},
		{
			name:   "success - stored, nothing received",
			stored: full,
			header: http.Header{},

			assertErr:      require.NoError,
			wantTrustTuple: full,
			wantChanged:    false,
		},
		{
			name:   "success - stored, same issuer received",
			stored: full,
			header: trustTupleHeader("https://idp.example", "", ""),

			assertErr:      require.NoError,
			wantTrustTuple: full,
			wantChanged:    false,
		},
		{
			name:   "error - stored, different issuer received",
			stored: full,
			header: trustTupleHeader("https://idp.attacker.example", "device", "audience"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, ErrOIDCTrustTupleChanged, a...)
				require.ErrorContains(tt, err, `issuer: stored "https://idp.example", received "https://idp.attacker.example"`, a...)
			},
			wantTrustTuple: full,
			wantChanged:    false,
		},
		{
			name:   "error - stored, different client ID received",
			stored: full,
			header: trustTupleHeader("", "attacker", ""),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, ErrOIDCTrustTupleChanged, a...)
				require.ErrorContains(tt, err, `client ID: stored "device", received "attacker"`, a...)
				require.NotContains(tt, err.Error(), "issuer:", a...)
			},
			wantTrustTuple: full,
			wantChanged:    false,
		},
		{
			name:   "error - stored without audience, audience received",
			stored: OIDCTrustTuple{Issuer: "https://idp.example", ClientID: "device"},
			header: trustTupleHeader("https://idp.example", "device", "audience"),

			assertErr:      errassert.Contains(`audience: stored "", received "audience"`),
			wantTrustTuple: OIDCTrustTuple{Issuer: "https://idp.example", ClientID: "device"},
			wantChanged:    false,
		},
		{
			name:   "error - stored issuer only, client ID received",
			stored: OIDCTrustTuple{Issuer: "https://idp.example"},
			header: trustTupleHeader("https://idp.example", "device", ""),

			assertErr:      errassert.Is(ErrOIDCTrustTupleChanged),
			wantTrustTuple: OIDCTrustTuple{Issuer: "https://idp.example"},
			wantChanged:    false,
		},
		{
			name:   "error - stored client ID only, issuer received",
			stored: OIDCTrustTuple{ClientID: "device"},
			header: trustTupleHeader("https://idp.attacker.example", "device", ""),

			assertErr:      errassert.Is(ErrOIDCTrustTupleChanged),
			wantTrustTuple: OIDCTrustTuple{ClientID: "device"},
			wantChanged:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			trustTuple, changed, err := pinTrustTuple(tc.stored, tc.header)

			// Assert
			tc.assertErr(t, err, "unexpected error")
			require.Equal(t, tc.wantTrustTuple, trustTuple, "unexpected trust tuple")
			require.Equal(t, tc.wantChanged, changed, "unexpected changed flag")
		})
	}
}

func trustTupleHeader(issuer string, clientID string, audience string) http.Header {
	header := http.Header{}
	for name, value := range map[string]string{
		"X-OperationsCenter-OIDC-issuer":   issuer,
		"X-OperationsCenter-OIDC-clientid": clientID,
		"X-OperationsCenter-OIDC-audience": audience,
	} {
		if value != "" {
			header.Set(name, value)
		}
	}

	return header
}
