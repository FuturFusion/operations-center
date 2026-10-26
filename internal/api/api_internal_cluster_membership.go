package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cowsql/go-cowsql/cluster/db"
	cowsqlmembership "github.com/cowsql/go-cowsql/cluster/membership"
	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/client"
	clusterSvc "github.com/FuturFusion/operations-center/internal/cluster"
	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

func registerInternalClusterMembershipHandler(
	router Router,
	authenticator *authn.Authenticator,
	isClusterMember func(r *http.Request) bool,
	service clusterSvc.MembershipService,
	restart func(),
	serverSvc provisioning.ServerService,
) {
	handler := &membershipHandler{
		service:   service,
		restart:   restart,
		serverSvc: serverSvc,
	}

	forwardToLeader := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			enabled, err := cowsqlmembership.Enabled(service.Gateway().Node())
			if err != nil {
				_ = response.SmartError(err).Render(w)
				return
			}

			if !enabled {
				next(w, r)
				return
			}

			address, err := service.Gateway().LeaderAddress()
			if err != nil {
				_ = response.SmartError(err).Render(w)
				return
			}

			if address == service.Gateway().State().ClusterAddress() {
				next(w, r)
				return
			}

			clusterCert, err := service.Gateway().NetworkCert().PublicKeyX509()
			if err != nil {
				_ = response.SmartError(err).Render(w)
				return
			}

			certInfo, ok := service.Gateway().ServerCert().(*incustls.CertInfo)
			if !ok {
				_ = response.InternalError(fmt.Errorf("Invalid certificate info: %T", service.Gateway().ServerCert())).Render(w)
				return
			}

			c, err := client.New("https://"+address, client.WithClientCertificate(certInfo), client.WithForwardedIdentity(forwardedIdentity(r)), client.WithTrustedServerCertificate(clusterCert), client.WithDialTimeout(clusterSvc.MemberDialTimeout))
			if err != nil {
				_ = response.SmartError(err).Render(w)
				return
			}

			_ = response.ForwardedResponse(c, r).Render(w)
		}
	}

	requireClusterDB := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if service.Gateway().Cluster() == nil {
				_ = response.SmartError(api.StatusErrorf(http.StatusTooEarly, "Cluster database is still initializing")).Render(w)
				return
			}

			next(w, r)
		}
	}

	// Redirect joining members, since the leader has to see their own certificate.
	redirectToLeader := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			enabled, err := cowsqlmembership.Enabled(service.Gateway().Node())
			if err != nil {
				_ = response.SmartError(err).Render(w)
				return
			}

			if !enabled {
				next(w, r)
				return
			}

			address, err := service.Gateway().LeaderAddress()
			if err != nil {
				_ = response.SmartError(err).Render(w)
				return
			}

			if address == service.Gateway().State().ClusterAddress() {
				next(w, r)
				return
			}

			redirectURL := *r.URL
			redirectURL.Scheme = "https"
			redirectURL.Host = address

			http.Redirect(w, r, redirectURL.String(), http.StatusTemporaryRedirect)
		}
	}

	// Only the other cluster members manage the membership of this member.
	requireClusterMember := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !isClusterMember(r) {
				_ = response.Forbidden(nil).Render(w)
				return
			}

			next(w, r)
		}
	}

	leaderUntrusted := router.SubGroup("").AddMiddlewares(redirectToLeader)

	authenticated := router.SubGroup("").AddMiddlewares(authenticator.Middleware(), requireClusterMember)
	leaderAuthenticated := router.SubGroup("").AddMiddlewares(authenticator.Middleware(), requireClusterMember, requireClusterDB, forwardToLeader)

	// Joining requires the cluster database for the tokens and the members.
	leaderUntrusted = leaderUntrusted.AddMiddlewares(requireClusterDB)

	joinerAuthenticator := authn.New([]authn.Auther{service})
	leaderWithJoinerTrust := leaderUntrusted.SubGroup("").AddMiddlewares(joinerAuthenticator.Middleware())

	// Assign a cluster role.
	authenticated.HandleFunc("POST /:assign", response.With(handler.assignPost))
	// Handover the cluster member's role.
	leaderAuthenticated.HandleFunc("POST /:handover", response.With(handler.handoverPost))
	// Rebalance cluster roles.
	leaderAuthenticated.HandleFunc("POST /:rebalance", response.With(handler.rebalancePost))
	// Reload the shared config from the global database.
	authenticated.HandleFunc("POST /:reload-config", response.With(handler.reloadConfigPost))
	// Apply a cluster certificate updated by another cluster member.
	authenticated.HandleFunc("POST /:update-certificate", response.With(handler.updateCertificatePost))
	// Trust a supplied join token.
	leaderUntrusted.HandleFunc("POST /:trust", response.With(handler.trustPost))
	// Accept a new cluster member.
	leaderWithJoinerTrust.HandleFunc("POST /:accept", response.With(handler.acceptPost))
	// Reset this member to an empty standalone server after it has been removed from the cluster.
	authenticated.HandleFunc("POST /:reset", response.With(handler.resetPost))
}

func (m *membershipHandler) assignPost(r *http.Request) response.Response {
	req := membershipapi.InternalClusterPostAssignRequest{}

	// Parse the request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	// Quick checks.
	if len(req.RaftNodes) == 0 {
		return response.BadRequest(errors.New("No raft members provided"))
	}

	nodes := make([]db.RaftNode, len(req.RaftNodes))
	for i, raftNode := range req.RaftNodes {
		nodes[i].ID = raftNode.ID
		nodes[i].Address = raftNode.Address
		nodes[i].Role = db.RaftRole(raftNode.Role)
		nodes[i].Name = raftNode.Name
	}

	err = cowsqlmembership.Assign(m.service.Gateway(), nodes)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponse(true, nil)
}

func (m *membershipHandler) handoverPost(r *http.Request) response.Response {
	var req membershipapi.InternalClusterPostHandoverRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	if req.Address == "" {
		return response.BadRequest(errors.New("No address provided"))
	}

	err = m.service.Handover(r.Context(), req.Address)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) rebalancePost(r *http.Request) response.Response {
	err := m.service.Rebalance(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) reloadConfigPost(r *http.Request) response.Response {
	err := m.service.ReloadSharedConfig(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) updateCertificatePost(r *http.Request) response.Response {
	var req apisystem.CertificatePost

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	err = m.service.ApplyClusterCertificate(r.Context(), req.Certificate, req.Key)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) trustPost(r *http.Request) response.Response {
	joinToken := r.URL.Query().Get("token")

	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return response.BadRequest(errors.New("Request is not a TLS request"))
	}

	fingerprint := incustls.CertFingerprint(r.TLS.PeerCertificates[0])

	err := m.service.Trust(r.Context(), joinToken, fingerprint)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}

func (m *membershipHandler) resetPost(r *http.Request) response.Response {
	m.service.Reset(r.Context())
	m.restart()

	return response.EmptySyncResponse
}

func (m *membershipHandler) acceptPost(r *http.Request) response.Response {
	var req api.ClusterAcceptPost

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return response.BadRequest(err)
	}

	err = m.checkServerNameUnused(r.Context(), req.ServerName)
	if err != nil {
		return response.SmartError(err)
	}

	resp, err := m.service.Accept(r.Context(), req)
	if err != nil {
		return response.BadRequest(err)
	}

	return response.SyncResponse(true, resp)
}
