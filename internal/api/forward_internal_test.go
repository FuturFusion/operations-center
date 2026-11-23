package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
)

// trustedClientsStub is a membership service only answering TrustedClients.
type trustedClientsStub struct {
	clusteringSvc.MembershipService

	members map[string]x509.Certificate
}

func (s trustedClientsStub) TrustedClients() map[string]x509.Certificate {
	return s.members
}

// leaderStub is a membership service answering for the trusted members and the leader.
type leaderStub struct {
	trustedClientsStub

	leaderAddress string
	leaderErr     error
}

func (s leaderStub) LeaderAddress(context.Context) (string, error) {
	return s.leaderAddress, s.leaderErr
}

func TestForwarder_toLeader(t *testing.T) {
	t.Parallel()

	memberCert := generateCert(t)
	clientCert := generateCert(t)
	members := trustedClientsStub{members: map[string]x509.Certificate{incustls.CertFingerprint(memberCert): *memberCert}}

	tests := []struct {
		name          string
		leaderAddress string
		leaderErr     error
		cert          *x509.Certificate
		headers       map[string]string

		wantStatus  int
		wantMessage string
	}{
		{
			name:      "leader not known during an election",
			leaderErr: errors.New("No leader"),
			cert:      clientCert,

			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "The cluster leader is not known",
		},
		{
			name:          "forwarded to this member as the leader, while it is not",
			leaderAddress: "192.0.2.2:8443",
			cert:          memberCert,
			headers:       map[string]string{headerForwardedToLeader: "1"},

			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "The cluster leader changed while the request was forwarded",
		},
		{
			name:          "this member is the leader",
			leaderAddress: "",
			cert:          clientCert,
			headers:       map[string]string{headerForwardedToLeader: "1"},

			wantStatus: http.StatusOK,
		},
		{
			name:          "forwarded by a member to the leader, which this member is",
			leaderAddress: "",
			cert:          memberCert,
			headers:       map[string]string{headerForwardedToLeader: "1"},

			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			forward := forwarder{
				clustered:  func() bool { return true },
				membership: leaderStub{trustedClientsStub: members, leaderAddress: tc.leaderAddress, leaderErr: tc.leaderErr},
			}

			r := httptest.NewRequest(http.MethodPost, "/1.0/provisioning/clusters", http.NoBody)
			r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{tc.cert}}
			for key, value := range tc.headers {
				r.Header.Set(key, value)
			}

			resp := forward.toLeader(r)
			if tc.wantStatus == http.StatusOK {
				require.Nil(t, resp, "the request is handled by this member")
				return
			}

			require.NotNil(t, resp)

			rec := httptest.NewRecorder()
			require.NoError(t, resp.Render(rec))
			require.Equal(t, tc.wantStatus, rec.Code)
			require.Contains(t, rec.Body.String(), tc.wantMessage)
		})
	}
}

func TestMemberRequestChecks(t *testing.T) {
	t.Parallel()

	memberCert := generateCert(t)
	otherCert := generateCert(t)

	membership := trustedClientsStub{members: map[string]x509.Certificate{incustls.CertFingerprint(memberCert): *memberCert}}
	daemon := &Daemon{membershipSvc: membership}
	forward := forwarder{membership: membership}

	tests := []struct {
		name  string
		certs []*x509.Certificate

		wantMember bool
	}{
		{
			name:  "no certificate",
			certs: nil,

			wantMember: false,
		},
		{
			name:  "member certificate",
			certs: []*x509.Certificate{memberCert},

			wantMember: true,
		},
		{
			name:  "other certificate",
			certs: []*x509.Certificate{otherCert},

			wantMember: false,
		},
		{
			// Only the leaf is proven by the handshake, a member certificate elsewhere in the chain grants nothing.
			name:  "member certificate in chain",
			certs: []*x509.Certificate{otherCert, memberCert},

			wantMember: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			r.Header.Set(headerForwardedToMember, "1")
			r.TLS = &tls.ConnectionState{PeerCertificates: tc.certs}

			require.Equal(t, tc.wantMember, daemon.isClusterMemberRequest(r))
			require.Equal(t, tc.wantMember, forward.forwarded(r))
		})
	}
}

func generateCert(t *testing.T) *x509.Certificate {
	t.Helper()

	certPEM, _, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	cert, err := certificate.Decode(certPEM)
	require.NoError(t, err)

	return cert
}
