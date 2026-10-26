package api

import (
	"cmp"
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	incustls "github.com/lxc/incus/v7/shared/tls"

	ocClient "github.com/FuturFusion/operations-center/internal/client"
	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/util/logger"
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
		member:   member,
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

// otherMembers returns the other online cluster members, with the leader first.
func (f forwarder) otherMembers(ctx context.Context) ([]memberfiles.Member, error) {
	if !f.clustered() {
		return nil, nil
	}

	members, err := f.membership.GetMembersByID(ctx)
	if err != nil {
		return nil, err
	}

	leaderAddress, err := f.membership.LeaderAddress(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to check for cluster leadership: %w", err)
	}

	localAddress := config.GetLocalNetwork().ClusterAddress
	threshold := f.membership.HeartbeatOfflineThreshold()

	var others []memberfiles.Member
	for _, member := range members {
		if member.Address == localAddress || member.IsOffline(threshold) {
			continue
		}

		others = append(others, memberfiles.Member{ID: member.ID, Name: member.Name, Address: member.Address})
	}

	slices.SortFunc(others, func(a memberfiles.Member, b memberfiles.Member) int {
		if a.Address == leaderAddress {
			return -1
		}

		if b.Address == leaderAddress {
			return 1
		}

		return cmp.Compare(a.ID, b.ID)
	})

	return others, nil
}

// memberResponseTimeout limits waiting for a member to start answering a request.
const memberResponseTimeout = 5 * time.Second

// toFirstMember returns the response of the first other cluster member not answering r with "404 Not Found", notFound if none has the resource.
func (f forwarder) toFirstMember(r *http.Request, notFound error) response.Response {
	return &firstMemberResponse{forwarder: f, request: r, notFound: notFound}
}

// firstMemberResponse renders the response of the first cluster member having the requested resource.
type firstMemberResponse struct {
	forwarder forwarder
	request   *http.Request
	notFound  error
}

func (f *firstMemberResponse) Render(w http.ResponseWriter) error {
	members, err := f.forwarder.otherMembers(f.request.Context())
	if err != nil {
		return response.SmartError(err).Render(w)
	}

	for _, member := range members {
		resp, cancel, err := f.forwarder.probeForward(f.request, member)
		if err != nil {
			slog.DebugContext(f.request.Context(), "Skipping unreachable cluster member", slog.String("member", member.Name), logger.Err(err))
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			cancel()

			continue
		}

		return copyResponse(w, resp, cancel)
	}

	return response.SmartError(f.notFound).Render(w)
}

// Code returns the HTTP code.
func (f *firstMemberResponse) Code() int {
	return http.StatusOK
}

// copyResponse writes resp to w and ends it with cancel.
func copyResponse(w http.ResponseWriter, resp *http.Response, cancel context.CancelFunc) error {
	defer cancel()
	defer resp.Body.Close()

	for key := range resp.Header {
		w.Header().Set(key, resp.Header.Get(key))
	}

	w.WriteHeader(resp.StatusCode)

	_, err := io.Copy(w, resp.Body)

	return err
}

func (f *firstMemberResponse) String() string {
	return fmt.Sprintf("request to the first cluster member with %s", f.request.URL)
}

// probeMember reports whether another cluster member answers the request successfully.
func (f forwarder) probeMember(r *http.Request) bool {
	members, err := f.otherMembers(r.Context())
	if err != nil {
		slog.WarnContext(r.Context(), "Failed to get the cluster members", logger.Err(err))
		return false
	}

	for _, member := range members {
		resp, cancel, err := f.probeForward(r, member)
		if err != nil {
			continue
		}

		_ = resp.Body.Close()
		cancel()

		if resp.StatusCode == http.StatusOK {
			return true
		}
	}

	return false
}

// probeForward sends a copy of r with no body to member as a forwarded request.
func (f forwarder) probeForward(r *http.Request, member memberfiles.Member) (*http.Response, context.CancelFunc, error) {
	c, err := f.client(r, member)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(r.Context())

	req, err := http.NewRequestWithContext(ctx, r.Method, c.GetBaseAddr()+r.URL.RequestURI(), http.NoBody)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	for key := range r.Header {
		req.Header.Set(key, r.Header.Get(key))
	}

	req.Header.Set(headerForwardedToMember, "1")

	// Only waiting for the start of the response is limited, not reading it.
	timer := time.AfterFunc(memberResponseTimeout, cancel)

	resp, err := c.DoHTTP(req)
	if !timer.Stop() || err != nil {
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			err = context.DeadlineExceeded
		}

		return nil, nil, err
	}

	return resp, cancel, nil
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
