package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/cowsql/go-cowsql/cluster/db"
	cowsqlmembership "github.com/cowsql/go-cowsql/cluster/membership"
	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/client"
	clusterSvc "github.com/FuturFusion/operations-center/internal/cluster"
	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/internal/util/logger"
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

			tw := &headerTrackingWriter{ResponseWriter: w}
			err = response.ForwardedResponse(c, r).Render(tw)
			if err == nil {
				return
			}

			slog.WarnContext(r.Context(), "Failed to forward the request to the cluster leader", slog.String("leader", address), logger.Err(err))

			if !tw.written {
				_ = response.SmartError(domain.NewErrorf(domain.ErrUnavailable, "", "The cluster leader %q is unreachable", address).WithCause(err)).Render(w)
			}
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

}
