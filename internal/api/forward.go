package api

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"

	incustls "github.com/lxc/incus/v7/shared/tls"

	ocClient "github.com/FuturFusion/operations-center/internal/client"
	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

// headerForwardedToMember marks a request another cluster member forwarded to this member.
const headerForwardedToMember = "X-OperationsCenter-forwarded-to-member"

// headerForwardedToLeader marks a request another cluster member forwarded to this member as the cluster leader.
const headerForwardedToLeader = "X-OperationsCenter-forwarded-to-leader"

// forwarder forwards requests to the cluster member handling them.
type forwarder struct {
	clustered  func() bool
	membership clusteringSvc.MembershipService
	targets    memberfiles.Targets
}

// forwarded reports whether another cluster member forwarded r to this member.
func (f forwarder) forwarded(r *http.Request) bool {
	return r.Header.Get(headerForwardedToMember) != "" && f.fromMember(r)
}

// fromMember reports whether r is sent by another cluster member.
func (f forwarder) fromMember(r *http.Request) bool {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return false
	}

	// Only the leaf certificate is proven by the TLS handshake, the rest of the chain is unverified.
	cert := r.TLS.PeerCertificates[0]
	for _, member := range f.membership.TrustedClients() {
		if cert.Equal(&member) {
			return true
		}
	}

	return false
}

// forwardedToLeader reports whether a cluster member forwarded r to this member as the leader.
func (f forwarder) forwardedToLeader(r *http.Request) bool {
	return r.Header.Get(headerForwardedToLeader) != "" && f.fromMember(r)
}

// toLeader returns the response of the leader to r, nil if r is handled by this member.
func (f forwarder) toLeader(r *http.Request) response.Response {
	if !f.clustered() || f.forwarded(r) {
		return nil
	}

	leaderAddress, err := f.membership.LeaderAddress(r.Context())
	if err != nil {
		return response.SmartError(domain.NewErrorf(domain.ErrUnavailable, "", "The cluster leader is not known").
			WithHintf("Retry the request once the cluster has elected a leader.").
			WithCause(err))
	}

	if leaderAddress == config.GetLocalNetwork().ClusterAddress {
		return nil
	}

	// A member, which forwarded r to this member as the leader, gets no second forward, so r can not go back and forth during an election.
	if f.forwardedToLeader(r) {
		return response.SmartError(domain.NewErrorf(domain.ErrUnavailable, "", "The cluster leader changed while the request was forwarded").
			WithHintf("Retry the request."))
	}

	// Only a member sets the marker of a request forwarded to a member, never the client.
	r.Header.Del(headerForwardedToMember)
	r.Header.Set(headerForwardedToLeader, "1")

	c, err := f.client(r, memberfiles.Member{Address: leaderAddress})
	if err != nil {
		return response.SmartError(err)
	}

	return &forwardedToMemberResponse{
		Response: response.ForwardedResponse(c, r),
		unavailable: func(err error) error {
			return domain.NewErrorf(domain.ErrUnavailable, "", "The cluster leader %q is unreachable", leaderAddress).
				WithHintf("Retry the request once the cluster has elected a new leader.").
				WithCause(err)
		},
	}
}

// toMember returns the response of the cluster member memberID to r, nil if r is handled by this member.
func (f forwarder) toMember(r *http.Request, memberID int64) response.Response {
	if !f.clustered() || f.forwarded(r) {
		return nil
	}

	member, err := f.targets.Member(r.Context(), memberID)
	if err != nil {
		return response.SmartError(err)
	}

	if member.Local {
		return nil
	}

	r.Header.Set(headerForwardedToMember, "1")

	return f.forward(r, member)
}

func (f forwarder) forward(r *http.Request, member memberfiles.Member) response.Response {
	c, err := f.client(r, member)
	if err != nil {
		return response.SmartError(err)
	}

	return &forwardedToMemberResponse{
		Response: response.ForwardedResponse(c, r),
		unavailable: func(err error) error {
			return memberfiles.Unavailable(member, err)
		},
	}
}

// client returns a client to a cluster member with the identity of the original requestor included.
func (f forwarder) client(r *http.Request, member memberfiles.Member) (ocClient.OperationsCenterClient, error) {
	clusterCert, err := f.membership.Gateway().NetworkCert().PublicKeyX509()
	if err != nil {
		return ocClient.OperationsCenterClient{}, fmt.Errorf("Failed to parse cluster certificate: %w", err)
	}

	certInfo, ok := f.membership.Gateway().ServerCert().(*incustls.CertInfo)
	if !ok {
		//domain-errors:internal The server certificate is always loaded by the daemon itself.
		return ocClient.OperationsCenterClient{}, fmt.Errorf("Invalid certificate info: %T", f.membership.Gateway().ServerCert())
	}

	return ocClient.New("https://"+member.Address, ocClient.WithClientCertificate(certInfo), ocClient.WithTrustedServerCertificate(clusterCert), ocClient.WithForwardedIdentity(forwardedIdentity(r)), ocClient.WithDialTimeout(clusteringSvc.MemberDialTimeout))
}

// forwardedToMemberResponse reports a member not answering the forwarded request as unavailable.
type forwardedToMemberResponse struct {
	response.Response

	// unavailable returns the error reported for err, which kept the member from answering.
	unavailable func(err error) error
}

func (f *forwardedToMemberResponse) Render(w http.ResponseWriter) error {
	tw := &headerTrackingWriter{ResponseWriter: w}

	err := f.Response.Render(tw)
	if err != nil && !tw.written {
		var serverErr *ocClient.ServerError
		if errors.As(err, &serverErr) {
			return response.SmartError(err).Render(w)
		}

		return response.SmartError(f.unavailable(err)).Render(w)
	}

	return err
}

// headerTrackingWriter tracks whether the response has been started.
type headerTrackingWriter struct {
	http.ResponseWriter

	written bool
}

func (h *headerTrackingWriter) WriteHeader(statusCode int) {
	h.written = true
	h.ResponseWriter.WriteHeader(statusCode)
}

func (h *headerTrackingWriter) Write(b []byte) (int, error) {
	h.written = true
	return h.ResponseWriter.Write(b)
}

// forwardedIdentity returns the identity of the client of r to pass on with a forwarded request.
func forwardedIdentity(r *http.Request) ocClient.ForwardedIdentity {
	ctx := r.Context()
	identity := ocClient.ForwardedIdentity{Address: r.RemoteAddr}

	authenticated, ok := ctx.Value(authn.CtxAuthenticated).(bool)
	if ok && authenticated {
		username, ok := ctx.Value(authn.CtxUsername).(string)
		if ok {
			identity.Username = username
		}

		protocol, ok := ctx.Value(authn.CtxProtocol).(string)
		if ok {
			identity.Protocol = protocol
		}
	}

	// A request forwarded by another member keeps the identity of its client.
	address, ok := ctx.Value(authn.CtxForwardedAddress).(string)
	if ok {
		identity.Address = address
	}

	identity.Certificate = forwardingCertificate(r)

	return identity
}

// forwardingCertificate returns the client certificate of r, or the one passed on if r is forwarded itself.
func forwardingCertificate(r *http.Request) *x509.Certificate {
	cert, ok := r.Context().Value(authn.CtxForwardedCertificate).(*x509.Certificate)
	if ok {
		return cert
	}

	_, forwarded := r.Context().Value(authn.CtxForwardedAddress).(string)
	if forwarded || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil
	}

	return r.TLS.PeerCertificates[0]
}
