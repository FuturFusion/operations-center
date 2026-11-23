package cluster

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	cowsqlcluster "github.com/cowsql/go-cowsql/cluster"
	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestMembership_CreateToken_invalidExpiry(t *testing.T) {
	t.Parallel()

	for _, expiry := range []string{"0s", "-1h", "soon"} {
		t.Run(expiry, func(t *testing.T) {
			t.Parallel()

			err := (&membership{}).CreateToken(t.Context(), api.ClusterTokenPost{ServerName: "joiner", Expiry: expiry})

			var verr domain.ErrValidation
			require.ErrorAs(t, err, &verr)
		})
	}
}

// tokenRepoStub is a join token repo holding its tokens in memory.
type tokenRepoStub struct {
	tokens map[string]*entities.ClusterJoinToken
}

func (s *tokenRepoStub) GetTokens(context.Context) ([]entities.ClusterJoinToken, error) {
	tokens := make([]entities.ClusterJoinToken, 0, len(s.tokens))
	for _, token := range s.tokens {
		tokens = append(tokens, *token)
	}

	return tokens, nil
}

func (s *tokenRepoStub) GetTokenByServerName(_ context.Context, serverName string) (*entities.ClusterJoinToken, error) {
	token, ok := s.tokens[serverName]
	if !ok {
		return nil, domain.NewErrorf(domain.ErrNotFound, "", "No join token for server %q", serverName)
	}

	return token, nil
}

func (s *tokenRepoStub) CreateToken(_ context.Context, token entities.ClusterJoinToken) error {
	s.tokens[token.ServerName] = &token
	return nil
}

func (s *tokenRepoStub) SetJoinerFingerprint(_ context.Context, serverName string, fingerprint string) error {
	token, ok := s.tokens[serverName]
	if !ok {
		return domain.NewErrorf(domain.ErrNotFound, "", "No join token for server %q", serverName)
	}

	token.JoinerFingerprint = fingerprint

	return nil
}

func (s *tokenRepoStub) DeleteTokenByName(_ context.Context, serverName string) error {
	delete(s.tokens, serverName)
	return nil
}

// tokenGateway is a gateway only telling whether this member is the leader.
type tokenGateway struct {
	cowsqlcluster.Gateway

	isLeader atomic.Bool
}

func (g *tokenGateway) IsLeader(context.Context) (bool, error) {
	return g.isLeader.Load(), nil
}

func (g *tokenGateway) RaftNode() *cowsqldb.RaftNode {
	return &cowsqldb.RaftNode{Name: "m1", Address: "192.0.2.1:8443"}
}

// joinerRequest returns a request presenting the certificate cert.
func joinerRequest(t *testing.T, cert *x509.Certificate) *http.Request {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/internal/membership/:accept", nil)
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}

	return r
}

func TestMembership_Trust(t *testing.T) {
	t.Parallel()

	certPEM, _, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)

	joinerCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	fingerprint := incustls.CertFingerprint(joinerCert)

	repo := &tokenRepoStub{tokens: map[string]*entities.ClusterJoinToken{
		"m2": {ServerName: "m2", Secret: "secret", ExpirationDate: time.Now().Add(time.Hour)},
	}}

	g := &tokenGateway{}
	g.isLeader.Store(true)

	m := &membership{gateway: g}
	m.tokenRepo.Store(func() *JoinTokenRepo { var r JoinTokenRepo = repo; return &r }())

	token := func(name string, secret string) string {
		s, err := api.ClusterJoinToken{ServerName: name, Secret: secret}.String()
		require.NoError(t, err)

		return s
	}

	// An unknown name and a wrong secret are refused alike, without telling them apart.
	errUnknown := m.Trust(t.Context(), token("unknown", "secret"), fingerprint)
	require.ErrorIs(t, errUnknown, domain.ErrNotAuthorized)

	errSecret := m.Trust(t.Context(), token("m2", "wrong"), fingerprint)
	require.ErrorIs(t, errSecret, domain.ErrNotAuthorized)
	require.Equal(t, domain.UserMessage(errUnknown), domain.UserMessage(errSecret))

	// Nothing is trusted so far.
	trusted, _, _, err := m.Auth(httptest.NewRecorder(), joinerRequest(t, joinerCert))
	require.NoError(t, err)
	require.False(t, trusted)

	// The secret records the joiner with its token, so any leader trusts the joiner afterwards.
	require.NoError(t, m.Trust(t.Context(), token("m2", "secret"), fingerprint))
	require.Equal(t, fingerprint, repo.tokens["m2"].JoinerFingerprint)

	trusted, _, protocol, err := m.Auth(httptest.NewRecorder(), joinerRequest(t, joinerCert))
	require.NoError(t, err)
	require.True(t, trusted)
	require.Equal(t, api.AuthenticationMethodTLS, protocol)

	// Only the leader trusts joiners.
	g.isLeader.Store(false)
	_, _, _, err = m.Auth(httptest.NewRecorder(), joinerRequest(t, joinerCert))
	require.Error(t, err)

	// A malformed token is invalid rather than unknown.
	g.isLeader.Store(true)
	err = m.Trust(t.Context(), "invalid", fingerprint)

	var verr domain.ErrValidation
	require.ErrorAs(t, err, &verr)
}
