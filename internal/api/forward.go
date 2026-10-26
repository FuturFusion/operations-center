package api

import (
	"crypto/x509"
	"fmt"
	"net/http"

	incustls "github.com/lxc/incus/v7/shared/tls"

	ocClient "github.com/FuturFusion/operations-center/internal/client"
	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/util/response"
)

// headerForwardedToMember marks a request another cluster member forwarded to this member.
const headerForwardedToMember = "X-OperationsCenter-forwarded-to-member"

// forwarder forwards requests to the cluster member handling them.
type forwarder struct {
	clustered  func() bool
	membership clusteringSvc.MembershipService
	targets    memberfiles.Targets
}

// forwarded reports whether another cluster member forwarded r to this member.
func (f forwarder) forwarded(r *http.Request) bool {
	if r.Header.Get(headerForwardedToMember) == "" || r.TLS == nil {
		return false
	}

	members := f.membership.TrustedClients()
	for _, cert := range r.TLS.PeerCertificates {
		for _, member := range members {
			if cert.Equal(&member) {
				return true
			}
		}
	}

	return false
}

// toLeader returns the response of the leader to r, nil if r is handled by this member.
func (f forwarder) toLeader(r *http.Request) response.Response {
	if !f.clustered() || f.forwarded(r) {
		return nil
	}

	leaderAddress, err := f.membership.LeaderAddress(r.Context())
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to check for cluster leadership: %w", err))
	}

	if leaderAddress == config.GetLocalNetwork().ClusterAddress {
		return nil
	}

	return f.forward(r, memberfiles.Member{Name: "leader", Address: leaderAddress})
}

func (f forwarder) forward(r *http.Request, member memberfiles.Member) response.Response {
	clusterCert, err := f.membership.Gateway().NetworkCert().PublicKeyX509()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to parse cluster certificate: %w", err))
	}

	certInfo, ok := f.membership.Gateway().ServerCert().(*incustls.CertInfo)
	if !ok {
		return response.InternalError(fmt.Errorf("Invalid certificate info: %T", f.membership.Gateway().ServerCert()))
	}

	c, err := ocClient.New("https://"+member.Address, ocClient.WithClientCertificate(certInfo), ocClient.WithTrustedServerCertificate(clusterCert), ocClient.WithForwardedIdentity(forwardedIdentity(r)), ocClient.WithDialTimeout(clusteringSvc.MemberDialTimeout))
	if err != nil {
		return response.SmartError(err)
	}

	return &forwardedToMemberResponse{
		Response: response.ForwardedResponse(c, r),
		member:   member,
	}
}

// forwardedToMemberResponse reports a member not answering the forwarded request as unavailable.
type forwardedToMemberResponse struct {
	response.Response

	member memberfiles.Member
}

func (f *forwardedToMemberResponse) Render(w http.ResponseWriter) error {
	tw := &headerTrackingWriter{ResponseWriter: w}

	err := f.Response.Render(tw)
	if err != nil && !tw.written {
		return response.SmartError(memberfiles.Unavailable(f.member, err)).Render(w)
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
